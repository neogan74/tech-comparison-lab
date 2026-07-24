// Package zabbix implements the bench.Backend for Zabbix: trapper-item
// provisioning and history reads via the JSON-RPC API, plus metric ingest via
// the native Zabbix sender protocol on the trapper port (default 10051).
package zabbix

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// header is the 13-byte Zabbix protocol framing: "ZBXD" + flags(0x01) +
// datalen(uint32 LE) + reserved(uint32 LE). This is the standard
// (non-large-packet) frame accepted by zabbix-server for sender data.
const (
	headerLen  = 13
	flagZabbix = 0x01
)

var magic = []byte("ZBXD")

// senderValue is one (host, key) sample sent to the trapper.
type senderValue struct {
	Host  string `json:"host"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Clock int64  `json:"clock"`
	NS    int    `json:"ns"`
}

// senderRequest is the JSON body of a "sender data" packet.
type senderRequest struct {
	Request string        `json:"request"`
	Data    []senderValue `json:"data"`
	Clock   int64         `json:"clock"`
}

// senderResponse is the trapper's reply. Info carries a human string like
// "processed: 5; failed: 0; total: 5; seconds spent: 0.000042".
type senderResponse struct {
	Response string `json:"response"`
	Info     string `json:"info"`
}

// Sender pushes batches of values to a Zabbix trapper endpoint.
type Sender struct {
	addr    string
	timeout time.Duration
}

// NewSender targets a zabbix-server trapper at addr ("host:port",
// e.g. localhost:10051).
func NewSender(addr string) *Sender {
	return &Sender{addr: addr, timeout: 30 * time.Second}
}

// encode frames a JSON payload with the Zabbix header.
func encode(payload []byte) []byte {
	buf := make([]byte, 0, headerLen+len(payload))
	buf = append(buf, magic...)
	buf = append(buf, flagZabbix)
	var lenBuf [8]byte
	binary.LittleEndian.PutUint32(lenBuf[0:4], uint32(len(payload)))
	// bytes 4-7 are the reserved field, left zero.
	buf = append(buf, lenBuf[:]...)
	return append(buf, payload...)
}

// decode reads one framed response from r and returns the JSON payload.
func decode(r io.Reader) ([]byte, error) {
	head := make([]byte, headerLen)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	if string(head[:4]) != "ZBXD" {
		return nil, fmt.Errorf("bad magic %q", head[:4])
	}
	n := binary.LittleEndian.Uint32(head[5:9])
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}

// send transmits values in a single connection and returns the parsed reply.
func (s *Sender) send(ctx context.Context, values []senderValue) (senderResponse, error) {
	var out senderResponse

	payload, err := json.Marshal(senderRequest{
		Request: "sender data",
		Data:    values,
		Clock:   time.Now().Unix(),
	})
	if err != nil {
		return out, fmt.Errorf("marshal request: %w", err)
	}

	d := net.Dialer{Timeout: s.timeout}
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return out, fmt.Errorf("dial %s: %w", s.addr, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.timeout))

	if _, err := conn.Write(encode(payload)); err != nil {
		return out, fmt.Errorf("write: %w", err)
	}

	body, err := decode(conn)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("unmarshal response %q: %w", body, err)
	}
	if out.Response != "success" {
		return out, fmt.Errorf("trapper returned response=%q info=%q", out.Response, out.Info)
	}
	return out, nil
}

// parseProcessed extracts the "processed: N" count from a trapper info string
// like "processed: 5; failed: 0; total: 5; seconds spent: 0.000042". It
// returns -1 if the field is absent or unparseable.
func parseProcessed(info string) int {
	for _, part := range strings.Split(info, ";") {
		part = strings.TrimSpace(part)
		if rest, ok := strings.CutPrefix(part, "processed:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				return -1
			}
			return n
		}
	}
	return -1
}
