package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func (s *server) handleToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var cookies []*network.CookieParam
	for _, cookie := range r.Cookies() {
		cookies = append(cookies, &network.CookieParam{
			Name:  cookie.Name,
			Value: cookie.Value,
			URL:   spotifyURL,
		})
	}

	body, err := s.getAccessTokenPayload(ctx, cookies)
	if err != nil {
		if !errors.Is(err, context.DeadlineExceeded) {
			slog.ErrorContext(ctx, "Failed to get access token payload", slog.Any("err", err))
		}
		http.Error(w, "Failed to get access token payload", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(body); err != nil {
		slog.ErrorContext(ctx, "Failed to write response", slog.Any("err", err))
	}
}

func (s *server) getAccessTokenPayload(rCtx context.Context, cookies []*network.CookieParam) ([]byte, error) {
	slog.DebugContext(rCtx, "Getting access token payload", slog.Int("cookieCount", len(cookies)))
	ctx, cancel := chromedp.NewContext(s.ctx)
	defer cancel()

	go func() {
		select {
		case <-rCtx.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	requestIDChan := make(chan network.RequestID, 1)
	var tokenRequestID network.RequestID

	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *network.EventResponseReceived:
			if !strings.HasPrefix(ev.Response.URL, spotifyTokenURL) {
				return
			}
			tokenRequestID = ev.RequestID
		case *network.EventLoadingFinished:
			if ev.RequestID == tokenRequestID {
				select {
				case requestIDChan <- ev.RequestID:
				default:
				}
			}
		}
	})

	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if len(cookies) == 0 {
				return nil
			}

			if err := network.SetCookies(cookies).Do(ctx); err != nil {
				return fmt.Errorf("failed to set cookies: %w", err)
			}

			return nil
		}),
		chromedp.Navigate(spotifyURL),
	); err != nil {
		return nil, err
	}

	var requestID network.RequestID
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case requestID = <-requestIDChan:
	}

	var body []byte
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		body, err = network.GetResponseBody(requestID).Do(ctx)
		return err
	})); err != nil {
		return nil, err
	}

	return body, nil
}
