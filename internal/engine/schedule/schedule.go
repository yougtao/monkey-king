package schedule

import (
	"context"
	"fmt"
	"github.com/xiaorui77/goutils/logx"
	"github.com/xiaorui77/goutils/wait"
	"github.com/xiaorui77/monker-king/internal/engine/api"
	"github.com/xiaorui77/monker-king/internal/engine/download"
	"github.com/xiaorui77/monker-king/internal/engine/schedule/task"
	"github.com/xiaorui77/monker-king/internal/storage"
	"github.com/xiaorui77/monker-king/internal/utils/domainutil"
	"github.com/xiaorui77/monker-king/pkg/model"
	"sort"
	"strconv"
	"time"
)

const (
	// Parallelism is maximum concurrent number of the same domain.
	Parallelism = 4

	// MaxDepth is max exploit depth of task
	MaxDepth = 3

	taskQueueSize = 100

	// TaskInterval task run interval
	TaskInterval = 1

	// DefaultTimeout is task default timeout
	DefaultTimeout = time.Second * 15
	MaxTimeout     = download.MaxTimeout
)

type Scheduler struct {
	parsing  api.Parsing
	download *download.Downloader
	store    storage.Storage

	taskQueue chan *task.Task
	// browser divide by domain
	browsers map[string]*Browser
}

func NewRunner(parsing api.Parsing, store storage.Storage) *Scheduler {
	return &Scheduler{
		parsing:   parsing,
		download:  download.NewDownloader(),
		taskQueue: make(chan *task.Task, taskQueueSize),
		browsers:  map[string]*Browser{},
		store:     store,
	}
}

// Run in Blocking mode
func (s *Scheduler) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Wait for all browsers to exit by themselves
			logx.Infof("[scheduler] ctx.done waiting for all browsers to stop")
			wait.WaitUntil(func() bool { return len(s.browsers) == 0 })
			logx.Debugf("[scheduler] all browsers has been stopped")
			s.close()
			logx.Infof("[scheduler] The scheduler has been stopped")
			return
		case t := <-s.taskQueue:
			t.SetState(task.StateInit)
			if _, ok := s.browsers[t.Domain]; !ok {
				s.browsers[t.Domain] = NewBrowser(s, t.Domain)
				go s.browsers[t.Domain].boot(ctx)
			}
			s.browsers[t.Domain].push(t)
		}
	}
}

func (s *Scheduler) AddTask(t *task.Task) error {
	if t == nil {
		return fmt.Errorf("task can not nil")
	}
	if t.Domain == "" {
		t.Domain = domainutil.CalDomain(t.Url)
	}
	if b, ok := s.browsers[t.Domain]; ok {
		if t.Depth > b.MaxDepth {
			return fmt.Errorf("browser[%s] max_depth is %d, but this task.depth is %d", t.Domain, b.MaxDepth, t.Depth)
		}
	}
	s.taskQueue <- t
	return nil
}

func (s *Scheduler) GetRows() []interface{} {
	now := time.Now()
	rows := make([]interface{}, 0, len(s.browsers))
	for _, domain := range s.browsers {
		ls := domain.list()
		// 默认排序: state,time
		sort.SliceStable(ls, func(i, j int) bool {
			if ls[i].State == ls[j].State {
				return ls[i].CreateTime.Unix() > ls[j].CreateTime.Unix()
			}
			return ls[i].State < ls[j].State
		})

		for _, t := range ls {
			row := &model.TaskRow{
				ID:     strconv.FormatUint(t.ID, 16),
				Name:   t.Name,
				Domain: domain.domain,
				State:  t.GetState(),
				URL:    t.Url,
			}
			if t.State == task.StateFailed && len(t.ErrDetails) > 0 {
				row.LastError = strconv.Itoa(t.ErrDetails[len(t.ErrDetails)-1].ErrCode)
			}
			if !t.StartTime.IsZero() {
				if t.EndTime.IsZero() {
					row.Age = fmt.Sprintf("%0.1fs", now.Sub(t.StartTime).Seconds())
				} else {
					row.Age = fmt.Sprintf("%0.1fs", t.EndTime.Sub(t.StartTime).Seconds())
				}
			}
			rows = append(rows, row)
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

func (s *Scheduler) DeleteTask(domain string, id uint64) bool {
	if b, ok := s.browsers[domain]; ok {
		if t := b.delete(id); t != nil {
			return true
		}
	}
	for _, b := range s.browsers {
		if t := b.delete(id); t != nil {
			return true
		}
	}
	return false
}

func (s *Scheduler) SetProcess(domain string, num int) {
	if b, ok := s.browsers[domain]; ok {
		b.SetProcess(num)
	}
}

func (s *Scheduler) GetTree(domain string) interface{} {
	if b, ok := s.browsers[domain]; ok {
		return b.tree()
	}
	return nil
}
