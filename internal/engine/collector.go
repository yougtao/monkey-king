package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/yougtao/goutils/logx"
	"github.com/yougtao/monker-king/internal/config"
	"github.com/yougtao/monker-king/internal/engine/schedule"
	"github.com/yougtao/monker-king/internal/storage"
	"github.com/yougtao/monker-king/internal/utils/localfile"
	"github.com/yougtao/monker-king/internal/view/model"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
)

type Collector struct {
	config    *config.Config
	store     storage.Store
	scheduler schedule.Runner

	// visited list
	visitedList map[string]bool

	// 抓取成功后回调
	register sync.Mutex
	// 获取到页面后的回调, 为了保证顺序, 所以采用list
	htmlCallbacks []HtmlCallbackContainer
}

func NewCollector(config *config.Config) (*Collector, error) {
	var store storage.Store
	var err error
	if config.Persistent {
		store, err = storage.NewRedisStore("127.0.0.1:6379")
		if err != nil {
			logx.Errorf("new collector failed: %v", err)
			return nil, errors.New("connect redis failed")
		}
	}

	runner := schedule.NewRunner(store)
	c := &Collector{
		config:    config,
		store:     store,
		scheduler: runner,

		visitedList:   map[string]bool{},
		htmlCallbacks: nil,
	}
	return c, nil
}

func (c *Collector) Run(ctx context.Context) {
	logx.Infof("[collector] Already running...")
	c.scheduler.Run(ctx)
}

// Visit 是对外的接口, 可以访问指定url
func (c *Collector) Visit(rawUrl string) error {
	if len(rawUrl) == 0 {
		return errors.New("rawUrl is empty")
	}

	u, err := url.Parse(rawUrl)
	if err != nil {
		logx.Warnf("[collector] new schedule failed with parse url(%v): %v", rawUrl, err)
		return err
	}
	return c.visit(u)
}

// Download 下载保存, todo: 待升级
func (c *Collector) Download(name, path string, urlRaw string) error {
	save := func(req *http.Request, resp *http.Response) error {
		defer func() {
			_ = resp.Body.Close()
		}()

		bs, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read resp.Body failed: %v", err)
		}
		logx.Debugf("save image %s to: %s", name, path)
		return localfile.SaveImage(bs, path, name)
	}

	u, err := url.Parse(urlRaw)
	if err != nil {
		logx.Warnf("[schedule] new schedule failed with parse url(%v): %v", urlRaw, err)
		return errors.New("未能识别的URL")
	}
	c.scheduler.AddTask(schedule.NewTask(name, u, nil, save), true)
	return nil
}

func (c *Collector) visit(u *url.URL) error {
	if len(u.Host) == 0 {
		logx.Warnf("[collector] visit url(%s) failed: rawUrl is invalid", u.String())
		return errors.New("rawUrl is invalid")
	}
	if err := c.filter(u); err != nil {
		logx.Warnf("[collector] filter url(%s) cause by: %v", u.String(), err)
		return err
	}

	c.AddTask(schedule.NewTask("", u, nil, c.onScrape))
	return nil
}

func (c *Collector) AddTask(t *schedule.Task) {
	if t == nil {
		return
	}
	logx.Debugf("[scrape] add Parser Task: %v", t.String())
	// c.ui.AddTaskRow(t)
	c.scheduler.AddTask(t, false)
}

// 处理抓取到的页面, todo: 对页面分类
func (c *Collector) onScrape(req *http.Request, resp *http.Response) error {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logx.Debugf("[collector] onScrape read body failed: %v", err)
		return fmt.Errorf("onScrape read body failed")
	}

	response := &Response{
		StatusCode: resp.StatusCode,
		Body:       body,
		Request: &Request{
			collector: c,
			baseURL:   req.URL, // todo: 该怎么设置
			URL:       req.URL,
		},
	}

	// 通过task下载get到页面后通过回调执行
	logx.Debugf("[collector] 下载完成, handle callback handleOnHtml[%v]", req.URL.String())
	c.handleOnHtml(response)
	c.recordVisit(req.URL.String())
	logx.Debugf("[collector] onScrape 分析完成, handleOnHtml[%v]", req.URL.String())
	return nil
}

// 借些页面, 处理回调
func (c *Collector) handleOnHtml(resp *Response) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewBuffer(resp.Body))
	if err != nil {
		logx.Debugf("parse html to document failed: %v", err)
		return
	}
	for _, callback := range c.htmlCallbacks {
		index := 1
		doc.Find(callback.Selector).Each(func(_ int, selection *goquery.Selection) {
			for _, node := range selection.Nodes {
				e := NewHTMLElement(resp, doc, selection, node, index)
				index++
				callback.fun(e)
			}
		})
	}
}

// @return ok: 是否继续
func (c *Collector) filter(u *url.URL) error {
	if c.isVisited(u.String()) {
		return fmt.Errorf("the URL has been browsed")
	}
	return nil
}

func (c *Collector) recordVisit(url string) {
	if c.config.Persistent {
		c.store.Visit(url)
	}
	c.visitedList[url] = true
}

func (c *Collector) isVisited(url string) bool {
	b, ok := c.visitedList[url]
	if ok {
		return b
	}
	return false
}

func (c *Collector) GetDataProducer() model.DataProducer {
	d, _ := c.scheduler.(*schedule.Scheduler)
	return d
}
