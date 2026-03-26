package modgearman

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/appscode/g2/client"
	"github.com/kdar/factorlog"
)

const (
	ServiceAnswerSize = 4
	HostAnswerSize    = 3
)

// Sendgearman starts the mod_gearman_worker program
func Sendgearman(build string) {
	config := sendgearmanInit(build)

	result := createResultFromArgs(config)
	if config.timeout <= 0 {
		config.timeout = 10
	}

	readCounter, sentCounter, errorCounter := sendgearmanLoop(config, result)
	errorText := ""
	if errorCounter > 0 {
		errorText = fmt.Sprintf(", %d result(s) failed", errorCounter)
	}
	log.Infof("Summary: %d result(s) read, %d result(s) sent successfully%s.", readCounter, sentCounter, errorText)
	if errorCounter > 0 {
		cleanExit(ExitCodeError)
	}
	cleanExit(0)
}

func sendgearmanInit(build string) *config {
	// reads the args, check if they are params, if so sends them to the configuration reader
	config, err := initConfiguration("send_gearman", build, printUsageSendGearman, checkForReasonableConfigSendGearman)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		cleanExit(ExitCodeUnknown)
	}

	// create the logger, everything logged until here gets printed to stdOut
	createLogger(config)
	log.SetOutput(os.Stderr)
	log.SetFormatter(factorlog.NewStdFormatter(`[%{Severity}] %{Message}`))

	// create the cipher
	key := getKey(config)
	myCipher = createCipher(key, config.encryption)

	if config.resultQueue == "" {
		config.resultQueue = "check_results"
	}

	return config
}

func readResults(config *config, baseResult *answer, resultsChan chan<- *answer) {
	defer close(resultsChan)

	read := make([]byte, 1024*1024*1024)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(read, cap(read))

	for {
		res := *baseResult

		if config.host == "" {
			if !readStdinLine(config, &res, scanner) {
				break
			}
		} else if config.message == "" {
			readStdinData(config, &res, scanner)
			log.Debugf("msg: %s", res.output)
		}

		resultsChan <- &res

		if config.host != "" {
			break
		}
	}
}

func trySendAnswerWithRetries(config *config, res *answer, clt *client.Client) (*client.Client, bool, error) {
	var err error
	var sent bool

	if res.serviceDescription != "" {
		log.Debugf("sending result for: %s - %s", res.hostName, res.serviceDescription)
	} else {
		log.Debugf("sending result for: %s", res.hostName)
	}

	for attempt := 0; attempt <= config.sendRetries; attempt++ {
		for _, a := range config.server {
			if clt == nil {
				log.Debugf("connecting to: %s", a)
			}
			clt, err = sendAnswer(clt, res, a, config.encryption, time.Duration(config.timeout)*time.Second)
			if err == nil {
				sent = true

				break
			}
			log.Debugf("connection failed: %v", err)
			if clt != nil {
				clt.Close()
				clt = nil
			}
		}

		if sent {
			break
		}
		if attempt < config.sendRetries {
			log.Debugf("failed to send result, retrying in %.2f s (attempt %d/%d)", config.sendRetryInterval, attempt+1, config.sendRetries)
			time.Sleep(time.Duration(config.sendRetryInterval * float64(time.Second)))
		}
	}

	return clt, sent, err
}

func sendgearmanLoop(config *config, result *answer) (readCounter, sentCounter, errorCounter int) {
	resultsChan := make(chan *answer, 100)
	go readResults(config, result, resultsChan)

	var clt *client.Client
	var err error
	var sent bool

	for res := range resultsChan {
		readCounter++

		if config.startTime <= 0 {
			res.startTime = float64(time.Now().Unix())
		}
		if config.finishTime <= 0 {
			res.finishTime = float64(time.Now().Unix())
		}

		clt, sent, err = trySendAnswerWithRetries(config, res, clt)

		if sent {
			sentCounter++
		} else {
			errorCounter++
			log.Errorf("failed to send result: %v", err)

			break
		}
	}

	if clt != nil {
		clt.Close()
	}

	// add remaining results from queue as errors
	remaining := len(resultsChan)
	readCounter += remaining
	errorCounter += remaining

	return readCounter, sentCounter, errorCounter
}

func readStdinLine(config *config, result *answer, scanner *bufio.Scanner) bool {
	timeout := time.AfterFunc(time.Duration(config.timeout)*time.Second, func() {
		log.Errorf("got no input after %s! Either send plugin output to stdin or use --message=.../--host=...",
			time.Duration(config.timeout)*time.Second)
		cleanExit(ExitCodeError)
	})
	if !scanner.Scan() {
		timeout.Stop()

		return false
	}
	timeout.Stop()
	if scanner.Err() != nil {
		log.Errorf("reading stdin failed: %s", scanner.Err().Error())
		cleanExit(ExitCodeError)
	}
	input := scanner.Text()
	if input == "" {
		return false
	}
	log.Debugf("input: %s", input)

	err := parseLine2Answer(config, result, input)
	if err != nil {
		log.Errorf("parsing stdin failed: %s", err.Error())
		cleanExit(ExitCodeError)
	}

	return true
}

func readStdinData(config *config, result *answer, scanner *bufio.Scanner) {
	timeout := time.AfterFunc(time.Duration(config.timeout)*time.Second, func() {
		log.Errorf("got no input after %s! Either send plugin output to stdin or use --message=...",
			time.Duration(config.timeout)*time.Second)
		cleanExit(ExitCodeError)
	})
	lines := make([]string, 0)
	for {
		cont := scanner.Scan()
		input := scanner.Text()
		lines = append(lines, input)
		err := scanner.Err()
		if !cont || (err != nil && errors.Is(err, io.EOF)) {
			timeout.Stop()
			result.output = strings.Join(lines, "\\n")

			return
		}
		if err != nil {
			timeout.Stop()
			log.Errorf("reading stdin failed: %s", err)
			cleanExit(ExitCodeError)
		}
	}
}

func createResultFromArgs(config *config) *answer {
	active := "passive"
	if config.active {
		active = "active"
	}

	result := &answer{
		hostName:           config.host,
		serviceDescription: config.service,
		returnCode:         config.returnCode,
		output:             config.message,
		active:             active,
		startTime:          config.startTime,
		finishTime:         config.finishTime,
		resultQueue:        config.resultQueue,
		source:             "send_gearman",
	}

	return result
}

func checkForReasonableConfigSendGearman(config *config) error {
	if len(config.server) == 0 {
		return fmt.Errorf("no server specified")
	}
	if config.encryption && config.key == "" && config.keyfile == "" {
		return fmt.Errorf("encryption enabled but no keys defined")
	}

	return nil
}

func parseLine2Answer(config *config, result *answer, input string) error {
	fields := strings.Split(input, config.delimiter)
	if len(fields) >= ServiceAnswerSize {
		// service result
		result.hostName = fields[0]
		result.serviceDescription = fields[1]
		result.returnCode = getInt(fields[2])
		result.output = fields[3]
	} else if len(fields) >= HostAnswerSize {
		// host result
		result.hostName = fields[0]
		result.serviceDescription = ""
		result.returnCode = getInt(fields[1])
		result.output = fields[2]
	}
	if result.hostName == "" {
		return fmt.Errorf("invalid data, no hostname parsed")
	}
	if result.hostName == "" {
		return fmt.Errorf("invalid data, no hostname parsed")
	}

	return nil
}

func printUsageSendGearman() {
	usage := `Usage: send_gearman [OPTION]...

send_gearman sends passive (and active) check results to a gearman daemon.

options:
             [ --debug=<lvl>                ]
             [ --help|-h                    ]

             [ --config=<configfile>        ]

             [ --server=<server>            ]

             [ --timeout=<timeout>          ]
             [ --delimiter=<delimiter>      ]

             [ --encryption=<yes|no>        ]
             [ --key=<string>               ]
             [ --keyfile=<file>             ]

             [ --host=<hostname>            ]
             [ --service=<servicename>      ]
             [ --result_queue=<queue>       ]
             [ --message|-m=<pluginoutput>  ]
             [ --returncode|-r=<returncode> ]
             [ --retries=<retries>          ]
             [ --retry-interval=<seconds>   ]

for sending active checks:
             [ --active                     ]
             [ --starttime=<unixtime>       ]
             [ --finishtime=<unixtime>      ]
             [ --latency=<seconds>          ]

plugin output is read from stdin unless --message is used.
Use --message when plugin has multiple lines of plugin output.

--timeout is used as timeout when reading results from stdin as well
as when sending results to the gearman daemon.

Note: When using a delimiter you may also submit one result
      for each line.
      Service Checks:
      <host_name>[tab]<svc_description>[tab]<return_code>[tab]<plugin_output>[newline]

      Host Checks:
      <host_name>[tab]<return_code>[tab]<plugin_output>[newline]
`
	fmt.Fprintln(os.Stdout, usage)

	cleanExit(ExitCodeUnknown)
}
