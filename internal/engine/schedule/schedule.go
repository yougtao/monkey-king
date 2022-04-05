package schedule

import (
	"context"
	"github.com/xiaorui77/goutils/wait"
	"github.com/xiaorui77/monker-king/internal/engine/download"
	"github.com/xiaorui77/monker-king/internal/engine/task"
	"github.com/xiaorui77/monker-king/internal/storage"
	"github.com/xiaorui77/monker-king/pkg/model"
	"sort"
	"time"
)

const (
	// Parallelism is maximum concurrent number of the same domain.
	Parallelism = 5

	// MaxDepth 为默认的最大深度
	MaxDepth = 3

	taskQueueSize = 100
)

type Scheduler struct {
	ctx      context.Context
	download *download.Downloader
	store    storage.Store

	taskQueue chan *task.Task
	// 以domain分开的队列
	browsers map[string]*DomainBrowser
}

func NewRunner(store storage.Store) *Scheduler {
	return &Scheduler{
		taskQueue: make(chan *task.Task, taskQueueSize),
		browsers:  map[string]*DomainBrowser{},
		store:     store,
	}
}

// Run in Blocking mode
func (s *Scheduler) Run(ctx context.Context) {
	s.ctx = ctx
	s.download = download.NewDownloader(ctx)

	for {
		select {
		case <-ctx.Done():
			s.close()
			wait.WaitUntil(func() bool { return len(s.browsers) == 0 })
			return
		case t := <-s.taskQueue:
			t.SetState(task.StateInit)
			if _, ok := s.browsers[t.Domain]; !ok {
				s.browsers[t.Domain] = NewDomainBrowser(s, t.Domain)
				go s.browsers[t.Domain].begin(ctx)
			}
			s.browsers[t.Domain].push(t)
		}
	}
}

func (s *Scheduler) AddTask(t *task.Task) {
	if t != nil {
		s.taskQueue <- t
	}
}

func (s *Scheduler) GetRows() []interface{} {
	now := time.Now()
	rows := make([]interface{}, 0, len(s.browsers))

	for _, domain := range s.browsers {
		ls := domain.list()
		// 默认排序: state,time
		sort.SliceStable(ls, func(i, j int) bool {
			if ls[i].State == ls[j].State {
				return ls[i].Time.Unix() > ls[j].Time.Unix()
			}
			return ls[i].State < ls[j].State
		})

		for _, t := range ls {
			rows = append(rows, &model.TaskRow{
				ID:     t.ID,
				Name:   t.Name,
				Domain: domain.domain,
				State:  task.StateStatus[t.State],
				URL:    t.Url.String(),
				Age:    now.Sub(t.Time).Truncate(time.Second).String(),
			})
		}
	}
	return rows
}

func (s *Scheduler) close() {
	// todo: 保存状态
}

func (s *Scheduler) GetTask(domain, task string) *task.Task {
	if b, ok := s.browsers[domain]; ok {
		return b.query(task)
	}
	return nil
}

func (s *Scheduler) DeleteTask(domain, task string) *task.Task {
	if b, ok := s.browsers[domain]; ok {
		return b.delete(task)
	}
	for _, b := range s.browsers {
		if t := b.delete(task); t != nil {
			return t
		}
	}
	return nil
}
