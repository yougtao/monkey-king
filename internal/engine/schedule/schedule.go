package schedule

import (
	"context"
	"github.com/yougtao/goutils/logx"
	"github.com/yougtao/goutils/wait"
	"github.com/yougtao/monker-king/internal/storage"
	"github.com/yougtao/monker-king/internal/view/model"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	// Parallelism is maximum concurrent number of the same host
	Parallelism = 2
)

// Runner 是一个运行器
type Runner interface {
	Run(ctx context.Context)
	AddTask(t *Task, priority bool)
}

type crawlerBrowser struct {
	// default client
	client    *http.Client
	cookiejar http.CookieJar
	ctx       context.Context

	// 以hostname分开的队列
	queue map[string]*DomainBrowser
	store storage.Store
}

func NewRunner(store storage.Store) *crawlerBrowser {
	jar, err := cookiejar.New(nil)
	if err != nil {
		logx.Errorf("new cookiejar failed: %v", err)
		return nil
	}
	return &crawlerBrowser{
		cookiejar: jar,
		client: &http.Client{
			Jar:     jar,
			Timeout: time.Second * 15,
		},

		queue: map[string]*DomainBrowser{},
		store: store,
	}
}

func (r *crawlerBrowser) Run(ctx context.Context) {
	r.ctx = ctx

	<-ctx.Done()
	wait.WaitUntil(func() bool { return len(r.queue) == 0 })
}

func (r *crawlerBrowser) AddTask(t *Task, priority bool) {
	if t == nil {
		return
	}

	if t.ID == 0 {
		t.ID = rand.Uint64()
	}

	host := t.Url.Host
	if _, ok := r.queue[host]; !ok {
		r.queue[host] = NewHostDomain(host)
		go r.queue[host].Schedule(r.ctx)
	}

	r.queue[host].Push(priority, t)
}

func (r *crawlerBrowser) GetRows() []interface{} {
	now := time.Now()
	rows := make([]interface{}, 0, len(r.queue))
	for _, domain := range r.queue {
		for _, t := range domain.List() {
			rows = append(rows, &model.TaskRow{
				ID:     t.ID,
				Name:   "",
				Domain: domain.domain,
				State:  TaskStateStatus[t.state],
				URL:    t.Url.String(),
				Age:    time.Since(now).String(),
			})
		}

	}
	return rows
}

func (r crawlerBrowser) list(host string, status int) {

}
