package agent

type AsyncTaskStateStore interface {
	Load() ([]AsyncTask, error)
	Save(tasks []AsyncTask) error
}
