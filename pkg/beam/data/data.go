package data

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

type StructuredStream interface {
	Send(Msg) error
	Receive() (Msg, error)
	Close() error
}

// A map of string arrays fits our needs for a structured message type neatly.
// It stores multiple values per key, so it can be used to carry scalars and arrays.
type Msg map[string][]string

func (m Msg) Add(k string, newValues...string) {
	values, exists := m[m.transformKey(k)]
	if !exists {
		m[m.transformKey(k)] = newValues
	} else {
		m[m.transformKey(k)] = append(values, newValues...)
	}
}

func (m Msg) Get(k string) string {
	if values, exists := m[m.transformKey(k)]; exists && len(values) >= 1 {
		return values[0]
	}
	return ""
}

func (m Msg) GetAll(k string) []string {
	if values, exists := m[m.transformKey(k)]; exists {
		return values
	}
	return nil
}

func (m Msg) String() string {
	return string(m.Bytes())
}

func (m Msg) ShortString() string {
	parts := make([]string, 0, len(m))
	for k, values := range m {
		shortVals := make([]string, 0, len(values))
		for _, v := range values {
			lines := strings.SplitN(v, "\n", 2)
			shortVal := lines[0]
			if len(lines) > 1 {
				shortVal = shortVal + "..."
			}
			shortVals = append(shortVals, shortVal)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, strings.Join(shortVals, ",")))
	}
	return strings.Join(parts, " ")
}

func (m Msg) Bytes() []byte {
	var buf bytes.Buffer
	m.WriteTo(&buf)
	return buf.Bytes()

}

func (m Msg) Exists(k string) bool {
	_, exists := m[m.transformKey(k)]
	return exists
}

func (m Msg) Set(k string, values ...string) {
	m[m.transformKey(k)] = values
}

func (m Msg) transformKey(k string) string {
	return strings.ToLower(k)
}

func (m Msg) Del(k string) {
	delete(m, m.transformKey(k))
}

func (m Msg) WriteTo(dst io.Writer) (written int64, err error) {
	var n int
	var nKey int
	for key, values := range m {
		for nValue, value := range values {
			if strings.ContainsRune(value, '\n') {
				// This snippet is adapted from the go-systemd package,
				// credits to the go-systemd authors:
				//
				// When the value contains a newline, we write:
				// - the variable name, followed by a newline
				// - the size (in 64bit little endian format)
				// - the data, followed by a newline
				//
				// FIXME: use spdy-style null-byte separation to send arrays
				// without repeating the keys.
				n, err = fmt.Fprintln(dst, key)
				written += int64(n)
				if err != nil {
					return
				}
				err = binary.Write(dst, binary.LittleEndian, uint64(len(value)))
				if err != nil {
					return
				}
				n, err = fmt.Fprintf(dst, "%s", value)
				written += int64(n)
				if err != nil {
					return
				}
			} else {
				n, err = fmt.Fprintf(dst, "%s=%s", key, value)
				written += int64(n)
				if err != nil {
					return
				}
			}
			if nKey != len(m) || nValue != len(values) {
				fmt.Fprintf(dst, "\n")
				written += 1
			}
		}
		nKey++
	}
	return
}

func (m Msg) ReadFrom(src io.Reader) (read int64, err error) {
	scanner := bufio.NewScanner(src)
	scanner.Split(scanMsg)
	for scanner.Scan() {
		entry := scanner.Text()
		nl := strings.Index(entry, "\n")
		if nl == -1 {
			eq := strings.Index(entry, "=")
			if eq == -1 {
				return 0, fmt.Errorf("invalid format at '%s'...", entry)
			}
			m.Add(entry[:eq], entry[eq+1:])
		} else {
			if len(entry) < nl+1+8 {
				return 0, fmt.Errorf("invalid format at '%s'...: expected %d bytes", entry, len(entry))
			}
			m.Add(entry[:nl], entry[nl+1+8:])
		}
		read += int64(len(entry))
		err = scanner.Err()
		if err != nil {
			return
		}
	}
	return
}

func scanMsg(data []byte, atEOF bool) (advance int, token []byte, err error) {
	s := string(data)
	if atEOF && len(s) == 0 {
		return 0, nil, nil
	}
	// Find the end of the current line
	eol := strings.Index(s, "\n")
	if eol == -1 {
		if atEOF {
			eol = len(s)
		} else {
			return 0, nil, nil
		}
	}
	// If the line is a simple text entry (<key>=<val>\n), we're done.
	if strings.ContainsRune(s[:eol], '=') {
		return eol + 1, data[:eol], nil
	}
	// Parse a binary entry (<key>\n<size><value>\n)
	if len(s) < eol+1+8 {
		if atEOF {
			return 0, nil, fmt.Errorf("invalid format: expected size of binary entry '%s', reached EOF", s[:eol])
		} else {
			// Request more data
			return 0, nil, nil
		}
	}
	var sizeUI64 uint64
	err = binary.Read(strings.NewReader(s[eol+1:]), binary.LittleEndian, &sizeUI64)
	if err != nil {
		return 0, data[:eol], fmt.Errorf("can't extract length of binary value '%s': %s", s[:eol], err)
	}
	size := int(sizeUI64)
	valueStart := eol + 1 + 8
	if len(s[valueStart:]) < size {
		if atEOF {
			return 0, nil, fmt.Errorf("invalid format: expected %d-byte value '%s', reached EOF", size, s[:eol])
		} else {
			return 0, nil, nil
		}
	}
	return valueStart + size, data[:valueStart+size], nil
}
