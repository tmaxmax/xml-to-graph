package cli

import (
	"fmt"
	"os"
	"sync"
)

var mu sync.Mutex

func printf(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	fmt.Fprintf(os.Stderr, format, args...)
}

func fatalf(format string, args ...interface{}) {
	printf(format, args...)
	os.Exit(1)
}
