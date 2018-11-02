package modgearman

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/appscode/g2/client"
	"github.com/kdar/factorlog"
)

// Sendgearman starts the mod_gearman_worker program
func Sendgearman(build string) {
	defer logPanicExit()

	config := sendgearmanInit(build)

	result := createResultFromArgs(config)
	if config.timeout <= 0 {
		config.timeout = 10
	}

	sendSuccess, resultsCounter, lastAddress, err := sendgearmanLoop(config, result)

	if !sendSuccess {
		logger.Errorf("failed to send back result: %s", err.Error())
		os.Exit(2)
	}
	logger.Infof("%d data packet(s) sent to host %s successfully.", resultsCounter, lastAddress)
	os.Exit(2)
}

func sendgearmanInit(build string) *configurationStruct {
	//reads the args, check if they are params, if so sends them to the configuration reader
	config, err := initConfiguration("mod_gearman_worker", build, printUsageSendGearman, checkForReasonableConfigSendGearman)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(3)
	}

	//create the logger, everything logged until here gets printed to stdOut
	createLogger(config)
	logger.SetOutput(os.Stderr)
	frmt := `[%{Severity}] %{Message}`
	logger.SetFormatter(factorlog.NewStdFormatter(frmt))

	//create the cipher
	key := getKey(config)
	myCipher = createCipher(key, config.encryption)

	if config.resultQueue == "" {
		config.resultQueue = "check_results"
	}

	return config
}

func sendgearmanLoop(config *configurationStruct, result *answer) (sendSuccess bool, resultsCounter int, lastAddress string, err error) {
	scanner := bufio.NewScanner(os.Stdin)
	read := make([]byte, 1024*1024*1024)
	scanner.Buffer(read, cap(read))

	// send result back to any server
	var c *client.Client
	for {
		if config.host == "" {
			// read package from stdin
			timeout := time.AfterFunc(time.Duration(config.timeout)*time.Second, func() {
				logger.Errorf("got no input after %d seconds! Either send plugin output to stdin or use --message=...", config.timeout)
				os.Exit(2)
			})
			if !scanner.Scan() {
				timeout.Stop()
				break
			}
			timeout.Stop()
			if scanner.Err() != nil {
				logger.Errorf("reading stdin failed: %s", scanner.Err().Error())
				os.Exit(2)
			}
			input := scanner.Text()
			if input == "" {
				break
			}
			if !parseLine2Answer(config, result, input) {
				break
			}
		}

		if config.startTime <= 0 {
			result.startTime = float64(time.Now().Unix())
		}
		if config.finishTime <= 0 {
			result.finishTime = float64(time.Now().Unix())
		}
		result.exitedOk = true
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
			logger.Debugf("connection failed: %s", err.Error())
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

func parseLine2Answer(config *configurationStruct, result *answer, input string) bool {
	fields := strings.Split(input, config.delimiter)
	if len(fields) >= 4 {
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
		return false
	}
	return true
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

	os.Exit(3)

}
