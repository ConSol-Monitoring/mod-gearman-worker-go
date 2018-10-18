package modgearman

import (
	"bytes"
	"fmt"
	"os/exec"
	"testing"
	time "time"

	"github.com/appscode/g2/client"
	"github.com/appscode/g2/pkg/runtime"
	libworker "github.com/appscode/g2/worker"
)

var resultChannel chan bool

func BenchmarkJobs(b *testing.B) {
	// prepare benchmark
	b.StopTimer()
	resultChannel = make(chan bool, 100)
	resultsTotal := 0
	config := configurationStruct{
		server:     []string{"127.0.0.1:54730"},
		key:        "testkey",
		encryption: true,
		hosts:      true,
		minWorker:  1,
		maxWorker:  1,
		jobTimeout: 10,
		debug:      0,
	}
	setDefaultValues(&config)
	disableLogging()
	cmd := exec.Command("gearmand", "--port", "54730", "--listen", "127.0.0.1", "--log-file", "stderr", "--verbose", "DEBUG")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		b.Skip(fmt.Sprintf("skipping test, could not start gearmand: %s.", err.Error()))
	}
	defer cmd.Process.Kill()

	key := getKey(&config)
	myCipher = createCipher(key, config.encryption)
	testData := fmt.Sprintf("type=host\nresult_queue=%s\nhost_name=%s\nstart_time=%f\ncore_time=%f\ncommand_line=%s\n",
		"results",
		"testhost",
		float64(time.Now().Unix()),
		float64(time.Now().Unix()),
		"/bin/pwd",
	)
	testJob := encodeBase64(encrypt([]byte(testData), key, true))

	var sender *client.Client
	for i := 0; i < 200; i++ {
		c, err := client.New("tcp4", "127.0.0.1:54730")
		if err == nil {
			sender = c
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// wait up to 10 seconds till gearmand is alive
	gearmandReady := false
	for i := 0; i < 200; i++ {
		_, err := sender.DoBg("test", testJob, runtime.JobNormal)
		if err == nil {
			gearmandReady = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !gearmandReady {
		b.Fatalf("failed to start gearmand within 10 seconds: %s\n%s", stdout.String(), stderr.String())
	}

	resultWorker := libworker.New(libworker.OneByOne)
	resultWorker.AddServer("tcp4", "127.0.0.1:54730")
	resultWorker.AddFunc("results", countResults, libworker.Unlimited)
	go resultWorker.Work()
	defer resultWorker.Close()

	shutdownChannel := make(chan bool)
	mainworker := newMainWorker(&config, getKey(&config))
	go func() {
		mainworker.managerWorkerLoop(shutdownChannel)
	}()
	defer func() {
		shutdownChannel <- true
		close(shutdownChannel)
	}()
	time.Sleep(1 * time.Second)

	b.StartTimer()
	go func() {
		for n := 0; n < b.N; n++ {
			// run e2e test
			_, err := sender.DoBg("host", testJob, runtime.JobNormal)
			if err != nil {
				b.Fatalf("sending job failed: %s", err.Error())
			}
		}
	}()
	for n := 0; n < b.N; n++ {
		<-resultChannel
		resultsTotal++
	}
	b.StopTimer()
}

func countResults(job libworker.Job) ([]byte, error) {
	resultChannel <- true
	return nil, nil
}
