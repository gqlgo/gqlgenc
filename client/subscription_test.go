package client

import (
	"context"
	"encoding/json/jsontext"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"
	"github.com/google/go-cmp/cmp"
)

// fakeGraphQLWSServer is a minimal graphql-transport-ws server for tests.
// It accepts a connection, expects connection_init, replies connection_ack,
// then on subscribe streams the given next payloads followed by complete.
type fakeGraphQLWSServer struct {
	nextPayloads         []string
	errorPayload         string
	pingBeforeAck        bool
	closeWithoutComplete bool
}

func (s fakeGraphQLWSServer) handler(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			Subprotocols: []string{graphqlTransportWSSubprotocol},
		})
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.CloseNow() //nolint:errcheck // best effort

		ctx := r.Context()

		init, err := readWSMessage(ctx, conn)
		if err != nil || init.Type != wsConnectionInit {
			t.Errorf("expected connection_init, got %q (err: %v)", init.Type, err)
			return
		}

		// connection_ack の前に ping を送り、クライアントが pong で応答することを確認する。
		if s.pingBeforeAck {
			if err := writeWSMessage(ctx, conn, wsMessage{Type: wsPing}); err != nil {
				t.Errorf("write ping: %v", err)
				return
			}
			pong, err := readWSMessage(ctx, conn)
			if err != nil || pong.Type != wsPong {
				t.Errorf("expected pong, got %q (err: %v)", pong.Type, err)
				return
			}
		}

		if err := writeWSMessage(ctx, conn, wsMessage{Type: wsConnectionAck}); err != nil {
			t.Errorf("write ack: %v", err)
			return
		}

		sub, err := readWSMessage(ctx, conn)
		if err != nil || sub.Type != wsSubscribe {
			t.Errorf("expected subscribe, got %q (err: %v)", sub.Type, err)
			return
		}

		for _, payload := range s.nextPayloads {
			msg := wsMessage{Type: wsNext, ID: sub.ID, Payload: jsontext.Value(payload)}
			if err := writeWSMessage(ctx, conn, msg); err != nil {
				return
			}
		}

		if s.errorPayload != "" {
			msg := wsMessage{Type: wsError, ID: sub.ID, Payload: jsontext.Value(s.errorPayload)}
			_ = writeWSMessage(ctx, conn, msg)
			return
		}

		// complete を送らずに WebSocket を正常クローズ(1000)するサーバーを模倣する。
		if s.closeWithoutComplete {
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return
		}

		_ = writeWSMessage(ctx, conn, wsMessage{Type: wsComplete, ID: sub.ID})
	}
}

func TestClient_Subscribe(t *testing.T) {
	t.Parallel()

	type result struct {
		Value int `json:"value"`
	}

	type vars struct {
		N int `json:"n"`
	}

	type fields struct {
		server fakeGraphQLWSServer
	}

	type want struct {
		values  []int
		wantErr bool
	}

	tests := []struct {
		name   string
		fields fields
		want   want
	}{
		{
			// next を順に受け取り complete で終了する
			name: "nextを順に受け取りcompleteで終了する",
			fields: fields{
				server: fakeGraphQLWSServer{
					nextPayloads: []string{
						`{"data":{"value":1}}`,
						`{"data":{"value":2}}`,
					},
				},
			},
			want: want{
				values: []int{1, 2},
			},
		},
		{
			// error メッセージを受け取るとエラーになる
			name: "errorメッセージでエラーになる",
			fields: fields{
				server: fakeGraphQLWSServer{
					nextPayloads: []string{`{"data":{"value":1}}`},
					errorPayload: `[{"message":"boom"}]`,
				},
			},
			want: want{
				values:  []int{1},
				wantErr: true,
			},
		},
		{
			// complete を送らずに正常クローズ(1000)された場合は完了として扱う
			name: "completeなしの正常クローズで完了として扱う",
			fields: fields{
				server: fakeGraphQLWSServer{
					nextPayloads: []string{
						`{"data":{"value":1}}`,
						`{"data":{"value":2}}`,
					},
					closeWithoutComplete: true,
				},
			},
			want: want{
				values: []int{1, 2},
			},
		},
		{
			// handshake 中に ack より前に ping が来ても pong で応答して継続する
			name: "handshake中のpingに応答して継続する",
			fields: fields{
				server: fakeGraphQLWSServer{
					nextPayloads:  []string{`{"data":{"value":1}}`},
					pingBeforeAck: true,
				},
			},
			want: want{
				values: []int{1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				httpServer := httptest.NewTestServer(t, tt.fields.server.handler(t))

				c := NewClient(httpServer.URL, WithHTTPClient(httpServer.Client()))

				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()

				var values []int
				var gotErr bool
				for res, err := range c.Subscribe(ctx, Operation[Subscription, vars, result]{Name: "Sub", Document: "subscription Sub { value }"}, vars{N: 1}) {
					if err != nil {
						gotErr = true
						break
					}
					values = append(values, res.Value)
				}

				if diff := cmp.Diff(tt.want.values, values); diff != "" {
					t.Errorf("values diff(-want +got): %s", diff)
				}
				if gotErr != tt.want.wantErr {
					t.Errorf("gotErr = %v, want %v", gotErr, tt.want.wantErr)
				}
			})
		})
	}
}

func TestDeriveWebSocketEndpoint(t *testing.T) {
	t.Parallel()

	type args struct {
		endpoint string
	}

	type want struct {
		endpoint string
	}

	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "httpはwsになる",
			args: args{endpoint: "http://example.com/graphql"},
			want: want{endpoint: "ws://example.com/graphql"},
		},
		{
			name: "httpsはwssになる",
			args: args{endpoint: "https://example.com/graphql"},
			want: want{endpoint: "wss://example.com/graphql"},
		},
		{
			name: "スキームが無い場合はそのまま",
			args: args{endpoint: "example.com/graphql"},
			want: want{endpoint: "example.com/graphql"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := deriveWebSocketEndpoint(tt.args.endpoint)

			if diff := cmp.Diff(tt.want.endpoint, got); diff != "" {
				t.Errorf("diff(-want +got): %s", diff)
			}
		})
	}
}

func TestWithWebSocketEndpoint(t *testing.T) {
	t.Parallel()

	const custom = "wss://ws.example.com/graphql"

	// http エンドポイントからは ws://example.com/graphql が導出されるが、
	// WithWebSocketEndpoint で上書きされることを確認する。
	c := NewClient("http://example.com/graphql", WithWebSocketEndpoint(custom))

	if c.wsEndpoint != custom {
		t.Errorf("wsEndpoint = %q, want %q", c.wsEndpoint, custom)
	}
}
