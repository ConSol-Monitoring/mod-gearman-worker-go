package modgearman

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/consol-monitoring/check_x"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/urfave/cli/v3"
)

type internalCheckPrometheus struct{}

func (chk *internalCheckPrometheus) Check(ctx context.Context, output *bytes.Buffer, args []string) int {
	// args passed to this function does not have the executable at first.
	var argsForCheck []string = make([]string, 0)
	argsForCheck = append(argsForCheck, "check_prometheus")
	argsForCheck = append(argsForCheck, args...)
	return check(argsForCheck, output)
}

type QueryEncoding int

const (
	Raw QueryEncoding = iota
	Base64
	Url
)

// Arguments to CheckPrometheus
var (
	// verbose flag is used during the http request interceptor to print out request details
	verbose bool = false
	// insecureSkipVerify will skip TLS certificate verification when set to true. It will be used when constructing http Transport
	insecureSkipVerify bool = false
	// cookies parsed into []*http.Cookie
	cookies []*http.Cookie
	// timestampFreshness is the ammount of second a result is treated as valid
	timestampFreshness int
	// base address of the prometheus, without the /api/v1/ at the end
	address           *url.URL
	timeout           int64
	warning           string
	critical          string
	queryString       string
	queryDecoded      string
	queryEncoding     QueryEncoding
	alias             string
	search            string
	replace           string
	label             string
	emptyQueryMessage string
	emptyQueryStatus  string
	// DefaultLabel is used if the given label is wrong
	defaultLabel string = "instance"
)

func getStatus(state string) check_x.State {
	switch state {
	case "OK":
		return check_x.OK
	case "WARNING":
		return check_x.Warning
	case "CRITICAL":
		return check_x.Critical
	default:
		return check_x.Unknown
	}
}

// The code is taken from a cli application where calling os.Exit in each invocation is normal
// We have to intercept the check_x.Exit() calls and use them instead
func stateReturn(state check_x.State, msg string, out io.Writer) int {
	fmt.Fprintf(out, "%s", msg)
	return state.Code
}

func check(args []string, out io.Writer) int {
	cmd := &cli.Command{
		Name:    "check_prometheus",
		Usage:   "Checks different prometheus stats as well the data itself",
		Version: "0.0.4",
		Flags: []cli.Flag{
			&cli.Int64Flag{
				Name:        "timeout",
				Aliases:     []string{"t"},
				Usage:       "Seconds till check returns unknown, 0 to disable",
				Value:       10,
				Destination: &timeout,
			},
			&cli.IntFlag{
				Name:        "data-age",
				Aliases:     []string{"f"},
				Usage:       "If the checked data is older then this in seconds, unknown will be returned. Set to 0 to disable.",
				Value:       300,
				Destination: &timestampFreshness,
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Turn the verbose mode on.",
				Value: false,
				Action: func(ctx context.Context, cmd *cli.Command, value bool) error {
					verbose = value
					return nil
				},
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "mode",
				Aliases: []string{"m"},
				Usage:   "check mode",
				Commands: []*cli.Command{
					{
						Name:        "ping",
						Aliases:     []string{"p"},
						HideHelp:    false,
						Usage:       "Returns the build informations",
						Description: `This check requires that the prometheus server itself is listed as target. Following query will be used: 'prometheus_build_info{job="prometheus"}'`,
						Action: func(ctx context.Context, cmd *cli.Command) error {
							ret := ping(address, out)
							if ret == 0 {
								return nil
							} else {
								return fmt.Errorf("Error when executing cli action")
							}
						},
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "address",
								Usage: "Prometheus address: Protocol + IP + Port.",
								Value: "http://localhost:9100",
								Action: func(ctx context.Context, cmd *cli.Command, value string) error {
									url, err := url.Parse(value)
									if err != nil {
										address = url
									}
									return err
								},
								Validator: func(value string) error {
									_, err := url.Parse(value)
									return err
								},
								ValidateDefaults: true,
							},
						},
					},

					{
						Name:     "query",
						Aliases:  []string{"q"},
						HideHelp: false,
						Usage:    "Checks collected data",
						Description: `Your Promqlquery has to return a vector / scalar / matrix result. The warning and critical values are applied to every value.
									Examples:
										Vector:
											check_prometheus mode query -q 'up'
										--> OK - Query: 'up'|'up{instance="192.168.99.101:9245", job="iapetos"}'=1;;;; 'up{instance="0.0.0.0:9091", job="prometheus"}'=1;;;;

										Scalar:
											check_prometheus mode query -q 'scalar(up{job="prometheus"})'
										--> OK - OK - Query: 'scalar(up{job="prometheus"})' returned: '1'|'scalar'=1;;;;

										Matrix:
											check_prometheus mode query -q 'http_requests_total{job="prometheus"}[5m]'
										--> OK - Query: 'http_requests_total{job="prometheus"}[5m]'

										Search and Replace:
											check_prometheus m query -q 'up' --search '.*job=\"(.*?)\".*' --replace '$1'
										--> OK - Query: 'up'|'prometheus'=1;;;; 'iapetos'=0;;;;

											check_prometheus m q -q '{handler="prometheus",quantile="0.99",job="prometheus",__name__=~"http_.*bytes"}' --search '.*__name__=\"(.*?)\".*' --replace '$1' -a 'http_in_out'
										--> OK - Alias: 'http_in_out'|'http_request_size_bytes'=296;;;; 'http_response_size_bytes'=5554;;;;

										Use Alias to generate output with label values:
											Assumption that your query returns a label "hostname" and "details".
											IMPORTANT: To be able to use the value in more advanced output formatting, we just add a label "value" with the current value to the list of labels.
											If the specified Alias string cannot be processed by the text/template engine, the Alias string will be printed 1:1.
											check_prometheus m q -a 'Hostname: {{.hostname}} - Details: {{.details}}' --search '.*' --replace 'error_state'  -q 'http_requests_total{job="prometheus"}' -w 0 -c 0
										--> Critical - Hostname: Server01 - Details: Error404|'error_state'=1;0;0;;

										Use Alias with an if/else clause and the use of xvalue:
											If xvalue is 1, we output UP, else we output DOWN
											check_prometheus m q --search '.*' --replace 'up' -q 'up{instance="SUPERHOST"}' -a 'Hostname: {{.hostname}} Is {{if eq .xvalue "1"}}UP{{else}}DOWN{{end}}.\n' -w 1: -c 1:
										--> OK - Hostname: SUPERHOST Is UP.\n|'up'=1;1:;1:;;

										List all available labels to be used with Alias:
											Just use -a '{{.}}' and the whole map with all labels will be printed.
											check_prometheus m q -q 'up{instance="SUPERHOST"}' -a '{{.}}'
											--> OK - map[__name__:up hostname:SUPERHOST instance:SUPERHOST job:snmp mib:RittalCMC xvalue:1]|'{__name__="up", hostname="SUPERHOST", instance="SUPERHOST", job="snmp", mib="RittalCMC"}'=1;;;;

										Use Different Message and Status code for queries that return no data.
											If you have a query that only returns data in an error condition you can use this flags to return a custom message and status code.
											check_prometheus m q -eqm 'All OK' -eqs 'OK'  -q 'http_requests_total{job="prometheus"}' -w 0 -c 0
											--> OK - All OK
											Without -eqm, -eqs
											check_prometheus m q -q 'http_requests_total{job="prometheus"}' -w 0 -c 0
											--> UNKNOWN - The given States do not contain an State

										`,
						Action: func(c context.Context, cmd *cli.Command) (err error) {
							ret := query(address, queryDecoded, warning, critical, alias, search, replace, emptyQueryMessage, getStatus(emptyQueryStatus), out)
							if ret == 0 {
								return nil
							} else {
								return fmt.Errorf("Error when executing cli action: %s", err.Error())
							}
						},
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "address",
								Usage: "Prometheus address: Protocol + IP + Port.",
								Value: "http://localhost:9100",
								Action: func(ctx context.Context, cmd *cli.Command, value string) error {
									url, err := url.Parse(value)
									if err == nil {
										address = url
									}
									return err
								},
								Validator: func(value string) error {
									_, err := url.Parse(value)
									return err
								},
								ValidateDefaults: true,
							},
							&cli.StringFlag{
								Name:        "q",
								Usage:       "Query to be executed",
								Destination: &queryString,
								Action: func(ctx context.Context, cmd *cli.Command, value string) error {
									queryString = value
									queryDecoded = value
									return nil
								},
							},
							&cli.StringFlag{
								Name:        "a",
								Usage:       "Alias, will replace the query within the output, if set. You can use go text/template syntax to output label values (only for vector results).",
								Destination: &alias,
							},
							&cli.StringFlag{
								Name:        "w",
								Usage:       "Warning value. Use nagios-plugin syntax here.",
								Destination: &warning,
							},
							&cli.StringFlag{
								Name:        "c",
								Usage:       "Critical value. Use nagios-plugin syntax here.",
								Destination: &critical,
							},
							&cli.StringFlag{
								Name:        "search",
								Usage:       "If this variable is set, the given Golang regex will be used to search and replace the result with the 'replace' flag content. This will be appied on the perflabels.",
								Destination: &search,
							},
							&cli.StringFlag{
								Name:        "replace",
								Usage:       "See search flag. If the 'search' flag is empty this flag will be ignored.",
								Destination: &replace,
							},
							&cli.BoolFlag{
								Name:        "insecure, k",
								Usage:       "Skip TLS certificate verification (insecure)",
								Destination: &insecureSkipVerify,
							},
							&cli.StringFlag{
								Name:  "cookie",
								Usage: "Cookie to send during the api request, in form '<name>=<value>' ",
								Action: func(ctx context.Context, cmd *cli.Command, value string) error {
									cookieKey := value[:strings.IndexRune(value, '=')]
									cookieValue := value[strings.IndexRune(value, '=')+1:]
									cookie := &http.Cookie{
										Name:     cookieKey,
										Value:    cookieValue,
										Path:     "/",
										SameSite: http.SameSiteLaxMode,
										MaxAge:   3600,
										Expires:  time.Now().Add(time.Hour),
									}
									cookies = append(cookies, cookie)
									return nil
								},
								Validator: func(value string) error {
									strings.Count(value, "=")
									if strings.Count(value, "=") != 1 {
										return fmt.Errorf("there should be exactly one '=' in the cookie definition")
									}
									cookieKey := value[:strings.IndexRune(value, '=')]
									cookieValue := value[strings.IndexRune(value, '=')+1:]
									if cookieKey == "" {
										return fmt.Errorf("cookie key cannot be empty")
									}
									if cookieValue == "" {
										return fmt.Errorf("cookie value cannot be empty")
									}
									if len(cookieValue) > 4096 {
										return fmt.Errorf("cookie value cannot be longer than 4096 characters")
									}

									return nil
								},
							},
							&cli.StringFlag{
								Name:        "eqm",
								Usage:       "Message if the query returns no data.",
								Destination: &emptyQueryMessage,
							},
							&cli.StringFlag{
								Name:        "eqs",
								Usage:       "Status if the query returns no data.",
								Destination: &emptyQueryStatus,
							},
							&cli.StringFlag{
								Name:  "query-encoding",
								Value: "raw",
								Usage: "Query encoding if query is given in encoded form. Supports 'raw', 'base64' and 'url' type encodings. Specify this parameter after the query.",
								Action: func(ctx context.Context, cmd *cli.Command, value string) error {
									if queryString == "" {
										return fmt.Errorf("query argument is empty, specify query-encoding after specifying query")
									}
									switch strings.ToLower(value) {
									case "raw":
										queryEncoding = Raw
										queryDecoded = queryString
									case "base64":
										queryEncoding = Base64
										bytes, err := base64.StdEncoding.DecodeString(queryString)
										if err != nil {
											return fmt.Errorf("base64 query decoding failed with error: %s", err.Error())
										}
										queryDecoded = string(bytes)
									case "url":
										queryEncoding = Url
										var err error
										queryDecoded, err = url.QueryUnescape(queryString)
										if err != nil {
											return fmt.Errorf("url query decoding failed with error: %s", err.Error())
										}
										return nil
									default:
										return fmt.Errorf("unknown query encoding, available values are 'raw', 'base64', 'url'")
									}
									return nil
								},
								Validator: func(value string) error {
									switch strings.ToLower(value) {
									case "raw":
										queryEncoding = Raw
									case "base64":
										queryEncoding = Base64
									case "url":
										queryEncoding = Url
									default:
										return fmt.Errorf("unknown query encoding, available values are 'raw', 'base64', 'url'")
									}
									return nil
								},
								ValidateDefaults: true,
							},
						},
					},

					{
						Name:        "targets_health",
						HideHelp:    false,
						Usage:       "Returns the health of the targets",
						Description: `The warning and critical thresholds are appied on the health_rate. The health_rate is calculted: sum(healthy) / sum(targets).`,
						Action: func(c context.Context, cmd *cli.Command) error {
							ret := targetsHealth(address, label, warning, critical, out)
							if ret == 0 {
								return nil
							} else {
								return fmt.Errorf("Error when executing cli action")
							}
						},
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "address",
								Usage: "Prometheus address: Protocol + IP + Port.",
								Value: "http://localhost:9100",
								Action: func(ctx context.Context, cmd *cli.Command, value string) error {
									url, err := url.Parse(value)
									if err != nil {
										address = url
									}
									return err
								},
								Validator: func(value string) error {
									_, err := url.Parse(value)
									return err
								},
								ValidateDefaults: true,
							},
							&cli.StringFlag{
								Name:        "w",
								Usage:       "Warning value. Use nagios-plugin syntax here.",
								Destination: &warning,
							},
							&cli.StringFlag{
								Name:        "c",
								Usage:       "Critical value. Use nagios-plugin syntax here.",
								Destination: &critical,
							},
							&cli.StringFlag{
								Name:        "l",
								Usage:       "Prometheus-Label, which will be used for the performance data label. By default job and instance should be available.",
								Destination: &label,
								Value:       defaultLabel,
							},
						},
					},
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), args); err != nil {
		return stateReturn(check_x.Unknown, fmt.Sprintf("error when running check: %s", err.Error()), out)
	}

	return stateOk
}

type prometheusInterceptor struct {
	next http.RoundTripper
}

func (i *prometheusInterceptor) RoundTrip(req *http.Request) (*http.Response, error) {
	if verbose {
		fmt.Printf("Sending %s request to %s\n", req.Method, req.URL.String())
		fmt.Printf("Request:\n%+v\n", req)
		fmt.Printf("Url:\n%+v\n", req.URL)
		fmt.Printf("Header:\n%+v\n", req.Header)

		// Read and print the body content
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				fmt.Printf("Error reading body: %v\n", err)
			} else {
				fmt.Printf("Body:\n%s\n", string(bodyBytes))
				// Restore the body for further processing
				req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
			}
		} else {
			fmt.Printf("Body is empty\n")
		}
	}

	// 2. Ensure the Content-Type is definitely set
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 3. Fix potential Idempotency-Key issues by removing it if it's nil
	// if val, ok := req.Header["Idempotency-Key"]; ok && val == nil {
	// 	delete(req.Header, "Idempotency-Key")
	// }

	return i.next.RoundTrip(req)
}

// Creates an prometheus v1 api client
func newAPIClientV1(address *url.URL) (v1.API, error) {
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureSkipVerify}

	interceptedTransport := &prometheusInterceptor{
		next: baseTransport,
	}

	httpClient := &http.Client{
		Transport: interceptedTransport,
	}

	// Initialize cookie jar only when Cookies are provided
	if len(cookies) > 0 {
		jar, _ := cookiejar.New(nil)
		httpClient.Jar = jar
		httpClient.Jar.SetCookies(address, cookies)
	}

	prometheusClient, err := api.NewClient(api.Config{
		Address: address.String(),
		Client:  httpClient,
	})

	if err != nil {
		return nil, err
	}

	return v1.NewAPI(prometheusClient), nil
}

// doAPIRequest does the http handling for an api request
func doAPIRequest(url *url.URL) ([]byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecureSkipVerify}

	httpClient := &http.Client{
		Transport: transport,
	}

	if len(cookies) > 0 {
		jar, _ := cookiejar.New(nil)
		httpClient.Jar = jar
		httpClient.Jar.SetCookies(url, cookies)
	}

	resp, err := httpClient.Get(url.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// checkTimestampFreshness tests if the data is still valid
func checkTimestampFreshness(timestamp model.Time, out io.Writer) int {
	return checkTimeFreshness(time.Unix(int64(timestamp), 0), out)
}

// checkTimeFreshness tests if the data is still valid
func checkTimeFreshness(timestamp time.Time, out io.Writer) int {
	if timestampFreshness == 0 {
		return stateOk
	}
	timeDiff := time.Since(timestamp)
	if int(timeDiff.Seconds()) > timestampFreshness {
		//check_x.Exit(check_x.Unknown, fmt.Sprintf("One of the scraped data exceed the freshness by %ds", int(timeDiff.Seconds())-TimestampFreshness))
		return stateReturn(check_x.Unknown, fmt.Sprintf("One of the scraped data exceed the freshness by %ds", int(timeDiff.Seconds())-timestampFreshness), out)
	}
	return stateOk
}

func query(address *url.URL, query, warning, critical, alias, search, replace, emptyQueryMessage string, emptyQueryStatus check_x.State, out io.Writer) int {
	warn, err := check_x.NewThreshold(warning)
	if err != nil {
		return stateUnknown
	}

	crit, err := check_x.NewThreshold(critical)
	if err != nil {
		return stateUnknown
	}
	var re *regexp.Regexp
	if search != "" {
		re, err = regexp.Compile(search)
		if err != nil {
			return stateUnknown
		}
	}

	apiClient, err := newAPIClientV1(address)
	if err != nil {
		return stateUnknown
	}

	result, _, err := apiClient.Query(context.TODO(), query, time.Now())
	if err != nil {
		return stateUnknown
	}

	switch result.Type() {
	case model.ValScalar:
		scalar := result.(*model.Scalar)
		scalarValue := float64(scalar.Value)
		if ret := checkTimestampFreshness(scalar.Timestamp, out); ret != stateOk {
			return ret
		}

		check_x.NewPerformanceData(replaceLabel("scalar", re, replace), scalarValue).Warn(warn).Crit(crit)
		state := check_x.Evaluator{Warning: warn, Critical: crit}.Evaluate(scalarValue)

		resultAsString := strconv.FormatFloat(scalarValue, 'f', -1, 64)
		if alias == "" {
			//check_x.Exit(state, fmt.Sprintf("Query: '%s' returned: '%s'", query, resultAsString))
			return stateReturn(state, fmt.Sprintf("Query: '%s' returned: '%s'", query, resultAsString), out)
		} else {
			//check_x.Exit(state, fmt.Sprintf("Alias: '%s' returned: '%s'", alias, resultAsString))
			return stateReturn(state, fmt.Sprintf("Alias: '%s' returned: '%s'", alias, resultAsString), out)
		}
	case model.ValVector:
		vector := result.(model.Vector)
		states := check_x.States{}
		var output string
		if len(vector) == 0 && emptyQueryMessage != "" {
			output = emptyQueryMessage
		} else if len(vector) == 0 {
			output = fmt.Sprintf("Query '%s' returned no data.", query)
		}
		if output != "" {
			//check_x.Exit(emptyQueryStatus, output)
			return stateReturn(emptyQueryStatus, output, out)
		}
		for _, sample := range vector {
			checkTimestampFreshness(sample.Timestamp, out)

			sampleValue := float64(sample.Value)
			check_x.NewPerformanceData(replaceLabel(model.LabelSet(sample.Metric).String(), re, replace), sampleValue).Warn(warn).Crit(crit)
			states = append(states, check_x.Evaluator{Warning: warn, Critical: crit}.Evaluate(sampleValue))
			output += expandAlias(alias, sample.Metric, sampleValue)
		}

		return evalStates(states, output, query, out)
	case model.ValMatrix:
		matrix := result.(model.Matrix)
		states := check_x.States{}
		for _, sampleStream := range matrix {
			for _, value := range sampleStream.Values {
				checkTimestampFreshness(value.Timestamp, out)
				states = append(states, check_x.Evaluator{Warning: warn, Critical: crit}.Evaluate(float64(value.Value)))
			}
		}

		return evalStates(states, alias, query, out)
	default:

		//check_x.Exit(check_x.Unknown, fmt.Sprintf("The query did not return a supported type(scalar, vector, matrix), instead: '%s'. Query: '%s'", result.Type().String(), query))
		return stateReturn(check_x.Unknown, fmt.Sprintf("The query did not return a supported type(scalar, vector, matrix), instead: '%s'. Query: '%s'", result.Type().String(), query), out)
	}
}

func expandAlias(alias string, labels model.Metric, value float64) string {
	_, err := template.New("Output").Parse(alias)
	var output string
	if err != nil {
		output = alias
	} else {
		labelMap := make(map[string]string)
		for label, value := range labels {
			var l = fmt.Sprintf("%v", label)
			var v = fmt.Sprintf("%v", value)
			labelMap[l] = v
		}
		labelMap["xvalue"] = fmt.Sprintf("%v", value)
		var rendered bytes.Buffer
		output = rendered.String()
	}

	return output
}

func replaceLabel(label string, re *regexp.Regexp, replace string) string {
	if re != nil {
		label = re.ReplaceAllString(label, replace)
	}

	return label
}

func evalStates(states check_x.States, alias, query string, out io.Writer) int {
	state, err := states.GetWorst()
	if err != nil {
		return stateUnknown
	}
	if alias == "" {
		//check_x.Exit(*state, fmt.Sprintf("Query: '%s'", query))
		return stateReturn(*state, fmt.Sprintf("Query: '%s'", query), out)
	} else {
		//check_x.Exit(*state, alias)
		return stateReturn(*state, alias, out)
	}
}

type buildInfo struct {
	Metric struct {
		Name      string `json:"__name__"`
		Branch    string `json:"branch"`
		Goversion string `json:"goversion"`
		Instance  string `json:"instance"`
		Job       string `json:"job"`
		Revision  string `json:"revision"`
		Version   string `json:"version"`
	} `json:"metric"`
	Value []interface{} `json:"value"`
}

// ping will fetch build information from the prometheus server
func ping(address *url.URL, out io.Writer) int {
	apiClient, err := newAPIClientV1(address)
	if err != nil {
		return stateUnknown
	}
	query := `prometheus_build_info{job="prometheus"}`
	startTime := time.Now()
	result, _, err := apiClient.Query(context.TODO(), query, time.Now())
	endTime := time.Now()
	if err != nil {
		return stateReturn(check_x.Unknown, fmt.Sprintf("error when querying data from client: %s", err.Error()), out)
	}
	vector := result.(model.Vector)
	if len(vector) != 1 {
		//return fmt.Errorf("the query '%s' did not return a vector with a single entry", query)
		return stateReturn(check_x.Warning, fmt.Sprintf("the query '%s' did not return a vector with a single entry", query), out)
	}
	sample := vector[0]
	checkTimestampFreshness(sample.Timestamp, out)
	jsonBytes, err := sample.MarshalJSON()
	if err != nil {
		return stateReturn(check_x.Unknown, "error when marshalling json data", out)
	}
	var dat buildInfo
	if err = json.Unmarshal(jsonBytes, &dat); err != nil {
		return stateReturn(check_x.Unknown, "error when unmarshalling json data", out)
	}
	check_x.NewPerformanceData("duration", endTime.Sub(startTime).Seconds()).Unit("s").Min(0)

	//check_x.Exit(check_x.OK, fmt.Sprintf("Version: %s, Instance %s", dat.Metric.Version, dat.Metric.Instance))
	return stateReturn(check_x.OK, fmt.Sprintf("Version: %s, Instance %s", dat.Metric.Version, dat.Metric.Instance), out)
}

type targets struct {
	Status string `json:"status"`
	Data   struct {
		ActiveTargets []struct {
			DiscoveredLabels struct {
				Address     string `json:"__address__"`
				MetricsPath string `json:"__metrics_path__"`
				Scheme      string `json:"__scheme__"`
				Job         string `json:"job"`
			} `json:"discoveredLabels"`
			Labels     map[string]string `json:"labels"`
			ScrapeURL  string            `json:"scrapeUrl"`
			LastError  string            `json:"lastError"`
			LastScrape time.Time         `json:"lastScrape"`
			Health     string            `json:"health"`
		} `json:"activeTargets"`
	} `json:"data"`
}

func getTargets(address *url.URL) (*targets, error) {
	u, err := url.Parse(address.String())
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, "/api/v1/targets")
	jsonBytes, err := doAPIRequest(u)
	if err != nil {
		return nil, err
	}
	var dat targets
	if err = json.Unmarshal(jsonBytes, &dat); err != nil {
		return nil, err
	}

	return &dat, nil
}

// TargetsHealth tests the health of the targets
func targetsHealth(address *url.URL, label, warning, critical string, out io.Writer) int {
	warn, err := check_x.NewThreshold(warning)
	if err != nil {
		return stateReturn(check_x.Unknown, "could not warning threshold from argument", out)
	}

	crit, err := check_x.NewThreshold(critical)
	if err != nil {
		return stateReturn(check_x.Unknown, "could not critical threshold from argument", out)
	}

	targets, err := getTargets(address)
	if err != nil {
		return stateReturn(check_x.Unknown, "error when getting the url out of address argument", out)
	}
	if (*targets).Status != "success" {
		// return fmt.Errorf("the API target returnstatus was %s", (*targets).Status)
		return stateReturn(check_x.Unknown, fmt.Sprintf("the API target returnstatus was %s", (*targets).Status), out)
	}
	msg := ""
	healthy := 0
	unhealthy := 0
	for _, target := range (*targets).Data.ActiveTargets {
		msg += fmt.Sprintf("Job: %s, Instance: %s, Health: %s, Last Error: %s\n", target.Labels["job"], target.Labels["instance"], target.Health, target.LastError)
		health := 0.0
		if target.Health != "up" {
			health = 1
			unhealthy += 1
		} else {
			healthy += 1
		}
		if val, ok := target.Labels[label]; ok {
			check_x.NewPerformanceData(val, health)
		} else {
			check_x.NewPerformanceData(target.Labels[defaultLabel], health)
		}
	}
	var healthRate float64
	sumTargets := float64(len((*targets).Data.ActiveTargets))
	if sumTargets == 0 {
		healthRate = 0
	} else {
		healthRate = float64(healthy) / sumTargets
	}
	check_x.NewPerformanceData("health_rate", healthRate).Warn(warn).Crit(crit).Min(0).Max(1)
	check_x.NewPerformanceData("targets", sumTargets).Min(0)
	state := check_x.Evaluator{Warning: warn, Critical: crit}.Evaluate(healthRate)

	//check_x.LongExit(state, fmt.Sprintf("There are %d healthy and %d unhealthy targets", healthy, unhealthy), msg)
	return stateReturn(state, fmt.Sprintf("There are %d healthy and %d unhealthy targets", healthy, unhealthy), out)
}
