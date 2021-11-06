package engine

import (
	"context"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/yougtao/monker-king/internal/engine/task"
	"github.com/yougtao/monker-king/internal/utils/localfile"
	"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

// Response is the representation of a HTTP response made by a Collector
type Response struct {
	// StatusCode is the status code of the Response
	StatusCode int
	// Body is the content of the Response
	Body []byte
	// Ctx is a context between a Request and a Response
	Ctx     context.Context
	Request *Request
}

type Request struct {
	collector *Collector
	baseURL   *url.URL
	URL       *url.URL
}

// Visit 继续浏览子页面
func (r *Request) Visit(url string) error {
	return r.collector.Visit(r.absoluteURL(url))
}

func (r *Request) Download(name, path string, url string) error {
	save := func(req *http.Request, resp *http.Response) error {
		defer func() {
			_ = resp.Body.Close()
		}()

		bs, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read resp.Body failed: %v", err)
		}
		return localfile.SaveImage(bs, path, name)
	}

	r.collector.tasks.AddTask(task.NewTask(url, save), true)
	return nil
}

func (r *Request) absoluteURL(u string) string {
	if strings.HasPrefix(u, "#") {
		return ""
	}
	var base *url.URL
	if r.baseURL != nil {
		base = r.baseURL
	} else {
		base = r.URL
	}
	absURL, err := base.Parse(u)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "//" {
		absURL.Scheme = r.URL.Scheme
	}
	return absURL.String()
}

type HTMLElement struct {
	Request  *Request
	Response *Response

	Doc   *goquery.Document
	DOM   *goquery.Selection
	Index int
	Node  *html.Node
	Attr  []html.Attribute
}

// NewHTMLElement 创建可操作的HTML结构
func NewHTMLElement(resp *Response, doc *goquery.Document, DOM *goquery.Selection, node *html.Node, index int) *HTMLElement {
	return &HTMLElement{

		Request:  resp.Request,
		Response: resp,

		Doc:   doc,
		DOM:   DOM,
		Index: index,
		Node:  node,
		Attr:  node.Attr,
	}
}

func (e HTMLElement) GetText(selector, def string) string {
	if str := e.Doc.Find(selector).Text(); str != "" {
		return str
	}
	return def
}
