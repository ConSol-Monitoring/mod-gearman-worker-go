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

const ServiceAnswerSize = 4

// Sendgearman starts the mod_gearman_worker program
func Sendgearman(build string) {
	config := sendgearmanInit(build)

	result := createResultFromArgs(config)
	if config.timeout <= 0 {
		config.timeout = 10
	}

	sendSuccess, resultsCounter, lastAddress, err := sendgearmanLoop(config, result)

	if !sendSuccess {
		logger.Errorf("failed to send back result: %s", err.Error())
		cleanExit(ExitCodeError)
	}
	logger.Infof("%d data packet(s) sent to host %s successfully.", resultsCounter, lastAddress)
	cleanExit(ExitCodeError)
}

func sendgearmanInit(build string) *configurationStruct {
	// reads the args, check if they are params, if so sends them to the configuration reader
	config, err := initConfiguration("send_gearman", build, printUsageSendGearman, checkForReasonableConfigSendGearman)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		cleanExit(ExitCodeUnknown)
	}

	// create the logger, everything logged until here gets printed to stdOut
	createLogger(config)
	logger.SetOutput(os.Stderr)
	logger.SetFormatter(factorlog.NewStdFormatter(`[%{Severity}] %{Message}`))

	// create the cipher
	key := getKey(config)
	myCipher = createCipher(key, config.encryption)

	if config.resultQueue == "" {
		config.resultQueue = "check_results"
	}

	return config
}

func sendgearmanLoop(config *configurationStruct, result *answer) (sendSuccess bool, resultsCounter int, lastAddress string, err error) {
	read := make([]byte, 1024*1024*1024)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(read, cap(read))

	// send result back to any server
	var c *client.Client
	for {
		// if no host is given from command line arguments, read from stdin
		if config.host == "" {
			// read all fields from stdin
			if !readStdinLine(config, result, scanner) {
				break
			}
		} else if config.message == "" {
			// read just the message from stdin
			readStdinData(config, result, scanner)
			logger.Debugf("msg: %s", result.output)
		}

		if config.startTime <= 0 {
			result.startTime = float64(time.Now().Unix())
		}
		if config.finishTime <= 0 {
			result.finishTime = float64(time.Now().Unix())
		}
		for _, a := range config.server {
			if c == nil {
				logger.Debugf("connecting to: %s", a)
				lastAddress = a
			}
			c, err = sendAnswer(c, result, a, config.encryption)
			if err == nil {
				resultsCounter++
				sendSuccess = true
				break
			}
			logger.Debugf("connection failed: %w", err)
			if c != nil {
				c.Close()
			}
		}

		if config.host != "" {
			return
		}
	}
	return
}

func readStdinLine(config *configurationStruct, result *answer, scanner *bufio.Scanner) bool {
	timeout := time.AfterFunc(time.Duration(config.timeout)*time.Second, func() {
		logger.Errorf("got no input after %d seconds! Either send plugin output to stdin or use --message=.../--host=...", config.timeout)
		cleanExit(ExitCodeError)
	})
	if !scanner.Scan() {
		timeout.Stop()
		return false
	}
	timeout.Stop()
	if scanner.Err() != nil {
		logger.Errorf("reading stdin failed: %s", scanner.Err().Error())
		cleanExit(ExitCodeError)
	}
	input := scanner.Text()
	if input == "" {
		return false
	}
	err := parseLine2Answer(config, result, input)
	return err == nil
}

func readStdinData(config *configurationStruct, result *answer, scanner *bufio.Scanner) {
	timeout := time.AfterFunc(time.Duration(config.timeout)*time.Second, func() {
		logger.Errorf("got no input after %d seconds! Either send plugin output to stdin or use --message=...", config.timeout)
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
			logger.Errorf("reading stdin failed: %s", err)
			cleanExit(ExitCodeError)
		}
	}
}

func createResultFromArgs(config *configurationStruct) *answer {
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

func checkForReasonableConfigSendGearman(config *configurationStruct) error {
	if len(config.server) == 0 {
		return fmt.Errorf("no server specified")
	}
	if config.encryption && config.key == "" && config.keyfile == "" {
		return fmt.Errorf("encryption enabled but no keys defined")
	}
	return nil
}

func parseLine2Answer(config *configurationStruct, result *answer, input string) error {
	fields := strings.Split(input, config.delimiter)
	if len(fields) >= ServiceAnswerSize {
		// service result
		result.hostName = fields[0]
		result.serviceDescription = fields[1]
		result.returnCode = getInt(fields[2])
		result.output = fields[3]
	} else {
		// host result
		result.hostName = fields[0]
		result.serviceDescription = ""
		result.returnCode = getInt(fields[1])
		result.output = fields[2]
	}
	if result.hostName == "" {
		return fmt.Errorf("invalid data, no hostname parsed")
	}
	return nil
}

func printUsageSendGearman() {
	fmt.Print(`Usage: send_gearman [OPTION]...

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

for sending active checks:
             [ --active                     ]
             [ --starttime=<unixtime>       ]
             [ --finishtime=<unixtime>      ]
             [ --latency=<seconds>          ]

plugin output is read from stdin unless --message is used.
Use --message when plugin has multiple lines of plugin output.

Note: When using a delimiter you may also submit one result
      for each line.
      Service Checks:
      <host_name>[tab]<svc_description>[tab]<return_code>[tab]<plugin_output>[newline]

      Host Checks:
      <host_name>[tab]<return_code>[tab]<plugin_output>[newline]
`)

	cleanExit(ExitCodeUnknown)
}
