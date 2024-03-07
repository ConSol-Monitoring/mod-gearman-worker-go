package modgearman

import (
	"fmt"

	libworker "github.com/appscode/g2/worker"
)

// creates a new status worker and returns a pointer to it
func newStatusWorker(configuration *config, mainWorker *mainWorker) *worker {
	return newWorker("status", configuration, mainWorker)
}

func (worker *worker) returnStatus(job libworker.Job) (result []byte, err error) {
	log.Debugf("status worker got a job: %s", job.Handle())

	err = job.Err()
	if err != nil {
		return
	}

	received := string(job.Data())
	log.Tracef("job data: %s", received)

	result = []byte(fmt.Sprintf("%s has %d worker and is working on %d jobs. Version: %s|worker=%d;;;%d;%d jobs=%dc",
		worker.config.identifier,
		len(worker.mainWorker.workerMap),
		worker.mainWorker.activeWorkers,
		VERSION,
		len(worker.mainWorker.workerMap),
		worker.config.minWorker,
		worker.config.maxWorker,
		worker.mainWorker.tasks,
	))

	return
}
