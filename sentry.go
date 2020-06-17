package sentry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sohaha/zlsgo/znet"
)

const valuesKey = "sentry"

type handler struct {
	repanic         bool
	waitForDelivery bool
	timeout         time.Duration
	PanicHandler    func(c *znet.Context, err error)
}

type Options struct {
	Dsn             string
	SampleRate      float64
	WaitForDelivery bool
	Timeout         time.Duration
	PanicHandler    func(c *znet.Context, err error)
}

// New returns a function
func New(options Options) (znet.HandlerFunc, error) {
	if options.Dsn == "" {
		return nil, errors.New("Sentry Dsn cannot be empty")
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:        options.Dsn,
		SampleRate: options.SampleRate,
	})
	if err != nil {
		return nil, fmt.Errorf("Sentry initialization failed: %v\n", err)
	}

	handler := handler{
		repanic:         false,
		timeout:         time.Second * 2,
		waitForDelivery: false,
	}

	if options.PanicHandler != nil {
		handler.PanicHandler = options.PanicHandler
	}

	if options.Timeout != 0 {
		handler.timeout = options.Timeout
	}

	if options.Timeout != 0 {
		handler.timeout = options.Timeout
	}

	if options.WaitForDelivery {
		handler.waitForDelivery = true
	}

	return handler.handle, nil
}

func (h *handler) handle(ctx *znet.Context) {
	hub := sentry.CurrentHub().Clone()
	hub.Scope().SetRequest(ctx.Request)
	ctx.WithValue(valuesKey, hub)
	defer h.recoverWithSentry(hub, ctx)
	ctx.Next()
}

func (h *handler) recoverWithSentry(hub *sentry.Hub, ctx *znet.Context) {
	if err := recover(); err != nil {
		fmt.Println(isBrokenPipeError(err), err)
		if !isBrokenPipeError(err) {
			eventID := hub.RecoverWithContext(
				context.WithValue(ctx.Request.Context(), sentry.RequestContextKey, ctx.Request),
				err,
			)
			if eventID != nil && h.waitForDelivery {
				hub.Flush(h.timeout)
			}
		}
		if h.PanicHandler != nil {
			errMsg, ok := err.(error)
			if !ok {
				errMsg = errors.New(fmt.Sprint(err))
			}
			h.PanicHandler(ctx, errMsg)
			return
		}
		panic(err)
	}
}

func isBrokenPipeError(err interface{}) bool {
	if netErr, ok := err.(*net.OpError); ok {
		if sysErr, ok := netErr.Err.(*os.SyscallError); ok {
			sysErrMsg := strings.ToLower(sysErr.Error())
			if strings.Contains(sysErrMsg, "broken pipe") ||
				strings.Contains(sysErrMsg, "connection reset by peer") {
				return true
			}
		}
	}
	return false
}

func GetHubFromContext(ctx *znet.Context) *sentry.Hub {
	if hub, ok := ctx.Value(valuesKey); ok {
		if hub, ok := hub.(*sentry.Hub); ok {
			return hub
		}
	}
	return nil
}
