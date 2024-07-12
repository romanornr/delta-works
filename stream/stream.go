package stream

import (
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
	"github.com/thrasher-corp/gocryptotrader/exchanges/stream"
	"github.com/thrasher-corp/gocryptotrader/exchanges/ticker"
)

var (
	ErrWebsocketNotSupported = errors.New("websocket not supported")
	ErrWebsocketNotEnabled   = errors.New("websocket not enabled")
)

func Stream(ctx context.Context, e exchange.IBotExchange) error {
	ws, err := OpenWebsocket(e)
	if err != nil {
		return ErrWebsocketNotSupported
	}

	for data := range ws.ToRoutine {
		err := handleData(e, data)
		if err != nil {
			logrus.Errorf("failed to handle data. Error: %s", err)
		}
	}
	panic("unexpected end of channel")
}

func OpenWebsocket(e exchange.IBotExchange) (*stream.Websocket, error) {
	if !e.SupportsWebsocket() {
		return nil, ErrWebsocketNotSupported
	}
	if !e.IsWebsocketEnabled() {
		return nil, ErrWebsocketNotEnabled
	}

	ws, err := e.GetWebsocket()
	if err != nil {
		return nil, err
	}

	if !ws.IsConnecting() && !ws.IsConnected() {
		err = ws.Connect()
		if err != nil {
			return nil, err
		}
		err = ws.FlushChannels()
		if err != nil {
			return nil, err
		}
	}
	return ws, nil
}

func handleData(e exchange.IBotExchange, data interface{}) error {
	switch x := data.(type) {
	case string:
		unhandledType(data, true)
	case error:
		return x
	case *ticker.Price:
		fmt.Printf("Price: %v\n", x)
		handleError("OnPrice", nil)
	default:
		handleError("OnUnrecognized", nil)
	}
	return nil
}

func handleError(method string, err error) {
	if err != nil {
		logrus.Errorf("failed to %s. Error: %s", method, err)
	}
}

func unhandledType(data interface{}, warn bool) {
	if warn {
		logrus.Warnf("unhandled type: %v", data)
	}
}
