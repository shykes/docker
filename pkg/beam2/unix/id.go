package unix

import (
	"fmt"
)

type IdCounter struct {
	odd bool // Should Ids be even or odd?
	last uint32
}

func (c *IdCounter) Register(id uint32) (uint32, error) {
	next, err := c.next()
	if err != nil {
		return 0, err
	}
	if id == 0 || id == next {
		c.last = next
		return next, nil
	}
	return 0, fmt.Errorf("invalid stream id %d: expected %d", id, next)
}

func (c *IdCounter) next() (uint32, error) {
	if c.last == 0 {
		if c.odd {
			return 2, nil
		} else {
			return 1, nil
		}
	}
        if c.last + 2 > 0xffffffff {
                return 0, fmt.Errorf("can't allocate new id: uint32 overflow")
        }
        return c.last + 2, nil
}

