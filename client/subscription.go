package client

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"

	"github.com/coder/websocket"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

// graphqlTransportWSSubprotocol is the WebSocket subprotocol name for the
// graphql-transport-ws protocol (https://github.com/enisdenjo/graphql-ws).
const graphqlTransportWSSubprotocol = "graphql-transport-ws"

// subscriptionReadLimit is the maximum size of a single WebSocket message.
// The coder/websocket default (32 KiB) is too small for many payloads.
const subscriptionReadLimit = 10 << 20 // 10 MiB

// graphql-transport-ws message types.
const (
	wsConnectionInit = "connection_init"
	wsConnectionAck  = "connection_ack"
	wsSubscribe      = "subscribe"
	wsNext           = "next"
	wsError          = "error"
	wsComplete       = "complete"
	wsPing           = "ping"
	wsPong           = "pong"
)

// wsMessage is a graphql-transport-ws envelope. Payload is left as raw JSON
// because its shape depends on the message type.
type wsMessage struct {
	Type    string         `json:"type"`
	ID      string         `json:"id,omitzero"`
	Payload jsontext.Value `json:"payload,omitzero"`
}

// Subscribe opens a GraphQL subscription for op over a WebSocket connection
// and returns an iterator that yields each decoded result until the server
// completes the subscription, the server reports an error, or ctx is done.
// Options apply only to this call and do not mutate c.
//
// The connection uses the graphql-transport-ws protocol. Each call opens its
// own connection, which is closed when the iteration stops (including when the
// consumer breaks out of the range loop). When ctx is cancelled the iterator
// stops without yielding the cancellation error.
//
// Even when an error is yielded for a result, that result may contain partial
// data, since a GraphQL "next" message can carry both data and errors.
func (c *Client) Subscribe[Vars, Res any](ctx context.Context, op Operation[Subscription, Vars, Res], vars Vars, options ...Option) iter.Seq2[*Res, error] {
	cc := c.withOptions(options...)

	return func(yield func(*Res, error) bool) {
		//nolint:bodyclose // coder/websocket does not require closing the handshake response body
		conn, _, err := websocket.Dial(ctx, cc.wsEndpoint, &websocket.DialOptions{
			HTTPClient:   cc.client,
			Subprotocols: []string{graphqlTransportWSSubprotocol},
		})
		if err != nil {
			yield(nil, fmt.Errorf("failed to dial websocket: %w", err))
			return
		}
		conn.SetReadLimit(subscriptionReadLimit)
		defer conn.CloseNow() //nolint:errcheck // best effort close on iteration end

		if err := cc.handshake(ctx, conn); err != nil {
			yield(nil, err)
			return
		}

		const subscriptionID = "1"
		if err := writeSubscribe(ctx, conn, subscriptionID, op.Name, op.Document, vars); err != nil {
			yield(nil, err)
			return
		}

		for {
			msg, err := readWSMessage(ctx, conn)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				// サーバーが complete を送らずに WebSocket を正常クローズした場合は、
				// エラーではなく完了として扱う。
				if status := websocket.CloseStatus(err); status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
					return
				}
				yield(nil, fmt.Errorf("failed to read message: %w", err))
				return
			}

			switch msg.Type {
			case wsNext:
				var res Res
				derr := unmarshalResponse(msg.Payload, &res)
				if !yield(&res, derr) {
					_ = writeWSMessage(ctx, conn, wsMessage{Type: wsComplete, ID: subscriptionID})
					return
				}
			case wsError:
				yield(nil, decodeSubscriptionError(msg.Payload))
				return
			case wsComplete:
				return
			case wsPing:
				if err := writeWSMessage(ctx, conn, wsMessage{Type: wsPong}); err != nil {
					yield(nil, fmt.Errorf("failed to send pong: %w", err))
					return
				}
			}
		}
	}
}

// handshake sends connection_init and waits for connection_ack.
func (c *Client) handshake(ctx context.Context, conn *websocket.Conn) error {
	if err := writeWSMessage(ctx, conn, wsMessage{Type: wsConnectionInit}); err != nil {
		return fmt.Errorf("failed to send connection_init: %w", err)
	}

	// connection_ack を待つ。graphql-transport-ws では ping はいつでも送られうるため、
	// ack より前に来た場合は pong で応答して待ち続ける。
	for {
		msg, err := readWSMessage(ctx, conn)
		if err != nil {
			return fmt.Errorf("failed to read connection_ack: %w", err)
		}
		switch msg.Type {
		case wsConnectionAck:
			return nil
		case wsPing:
			if err := writeWSMessage(ctx, conn, wsMessage{Type: wsPong}); err != nil {
				return fmt.Errorf("failed to send pong: %w", err)
			}
		default:
			return fmt.Errorf("expected connection_ack, got %q", msg.Type)
		}
	}
}

// writeSubscribe sends a subscribe message carrying the GraphQL request.
func writeSubscribe(ctx context.Context, conn *websocket.Conn, id, operationName, query string, vars any) error {
	payload, err := json.Marshal(&Request{
		Query:         query,
		Variables:     vars,
		OperationName: operationName,
	})
	if err != nil {
		return fmt.Errorf("failed to encode subscribe payload: %w", err)
	}

	return writeWSMessage(ctx, conn, wsMessage{Type: wsSubscribe, ID: id, Payload: payload})
}

// decodeSubscriptionError decodes an "error" message payload, which is a list
// of GraphQL errors.
func decodeSubscriptionError(payload jsontext.Value) error {
	var errs gqlerror.List
	if err := json.Unmarshal(payload, &errs); err != nil {
		return fmt.Errorf("failed to decode subscription error: %w", err)
	}
	if len(errs) == 0 {
		return errors.New("subscription error")
	}

	return errs
}

func writeWSMessage(ctx context.Context, conn *websocket.Conn, msg wsMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

func readWSMessage(ctx context.Context, conn *websocket.Conn) (wsMessage, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return wsMessage{}, fmt.Errorf("read websocket: %w", err)
	}

	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return wsMessage{}, fmt.Errorf("failed to decode message: %w", err)
	}

	return msg, nil
}
