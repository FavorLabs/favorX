package wiredtiger

/*
#cgo !windows CFLAGS: -I/usr/local/include
#cgo !windows LDFLAGS: -L/usr/local/lib -lwiredtiger
#cgo windows CFLAGS: -Ic:/wiredtiger/include
#cgo windows LDFLAGS: -Lc:/wiredtiger/lib -lwiredtiger

#include <stdlib.h>
#include <wiredtiger.h>
*/
import "C"

func Version() (version string, major int, minor int, patch int) {
	var a, b, c C.int

	version = C.GoString(C.wiredtiger_version(&a, &b, &c))
	major = int(a)
	minor = int(b)
	patch = int(c)

	return
}
