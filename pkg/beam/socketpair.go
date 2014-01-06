package beam

import (
	"fmt"
	"os"
	"syscall"
)

func Socketpair() (a *os.File, b *os.File, err error) {
	pair, err := syscall.Socketpair(syscall.AF_LOCAL, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("socketpair: %s", err)
	}
	a = os.NewFile(uintptr(pair[0]), fmt.Sprintf("%d", pair[0]))
	b = os.NewFile(uintptr(pair[1]), fmt.Sprintf("%d", pair[1]))
	return
}
