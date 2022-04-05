package download

import (
	"context"
	"github.com/xiaorui77/goutils/logx"
	"github.com/xiaorui77/monker-king/internal/engine/task"
	"github.com/xiaorui77/monker-king/internal/utils"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"
)

type Downloader struct {
	client *http.Client
	ctx    context.Context
}

func NewDownloader(ctx context.Context) *Downloader {
	jar, err := cookiejar.New(nil)
	if err != nil {
		logx.Errorf("[downloader] new cookiejar failed: %v", err)
		return nil
	}

	return &Downloader{
		ctx: ctx,
		client: &http.Client{
			Jar: jar,
			// The timeout includes connection time, any redirects, and reading the response body.
			// includes Dial、TLS handshake、Request、Resp.Headers、Resp.Body, excludes Idle
			Timeout: time.Second * 90,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   15 * time.Second,
					KeepAlive: 10 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:   true,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				TLSHandshakeTimeout: 15 * time.Second,
				IdleConnTimeout:     60 * time.Second,
			},
		},
	}
}

func (d *Downloader) Get(t *task.Task) {
	req, err := http.NewRequestWithContext(d.ctx, http.MethodGet, t.Url.String(), nil)
	if err != nil {
		logx.Errorf("[downloader] request.Get failed: %v")
		t.SetState(task.StateFail)
		return
	}
	d.beforeReq(req)

	resp, err := d.client.Do(req)
	if err != nil {
		logx.Warnf("[downloader] request.Do failed: %v", err)
		t.HandleOnResponseErr(resp, err)
		return
	}

	logx.Infof("[downloader] Task[%x] request.Do finish, begin task.HandleOnResponse()", t.ID)
	t.HandleOnResponse(req, resp)
}

func (d *Downloader) beforeReq(req *http.Request) {
	req.Header.Set(utils.UserAgentKey, utils.RandomUserAgent())

	// TODO: 待设定
	// req.Close = true
}
