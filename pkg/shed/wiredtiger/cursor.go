package wiredtiger

/*
#cgo LDFLAGS: -lwiredtiger -ltcmalloc -lsnappy

#include <stdlib.h>
#include <wiredtiger.h>

int wiredtiger_session_open_cursor(WT_SESSION *session, const char *uri, WT_CURSOR *to_dup, const char *config, WT_CURSOR **cursorp) {
	int ret = session->open_cursor(session, uri, to_dup, config, cursorp);
	if(ret)
		return ret;
	if (((*cursorp)->flags & WT_CURSTD_DUMP_JSON) == 0)
		(*cursorp)->flags |= WT_CURSTD_RAW;
	return 0;
}

int wiredtiger_cursor_reconfigure(WT_CURSOR *cursor, const char *config) {
	return cursor->reconfigure(cursor, config);
}

int wiredtiger_cursor_get_key(WT_CURSOR *cursor, WT_ITEM *v) {
	return cursor->get_key(cursor, v);
}

int wiredtiger_cursor_get_value(WT_CURSOR *cursor, WT_ITEM *v) {
	return cursor->get_value(cursor, v);
}

int wiredtiger_cursor_next(WT_CURSOR *cursor) {
	return cursor->next(cursor);
}

int wiredtiger_cursor_prev(WT_CURSOR *cursor) {
	return cursor->prev(cursor);
}

int wiredtiger_cursor_search(WT_CURSOR *cursor, const void *data, size_t size) {
	if (size != 0) {
		WT_ITEM key;
		key.data = data;
		key.size = size;
		cursor->set_key(cursor, &key);
	}
	return cursor->search(cursor);
}

int wiredtiger_cursor_search_near(WT_CURSOR *cursor, const void *data, size_t size, int *exactp) {
	if (size != 0) {
		WT_ITEM key;
		key.data = data;
		key.size = size;
		cursor->set_key(cursor, &key);
	}
	return cursor->search_near(cursor, exactp);
}

int wiredtiger_cursor_insert(WT_CURSOR *cursor, const void *key_data, size_t key_size, const void *val_data, size_t val_size) {
	if (key_size != 0) {
		WT_ITEM key;
		key.data = key_data;
		key.size = key_size;
		cursor->set_key(cursor, &key);
	}
	if (val_size != 0) {
		WT_ITEM value;
		value.data = val_data;
		value.size = val_size;
		cursor->set_value(cursor, &value);
	}
	return cursor->insert(cursor);
}

int wiredtiger_cursor_update(WT_CURSOR *cursor, const void *key_data, size_t key_size, const void *val_data, size_t val_size) {
	if (key_size != 0) {
		WT_ITEM key;
		key.data = key_data;
		key.size = key_size;
		cursor->set_key(cursor, &key);
	}
	if (val_size != 0) {
		WT_ITEM value;
		value.data = val_data;
		value.size = val_size;
		cursor->set_value(cursor, &value);
	}
	return cursor->update(cursor);
}

int wiredtiger_cursor_remove(WT_CURSOR *cursor, const void *data, size_t size) {
	if (size != 0) {
		WT_ITEM key;
		key.data = data;
		key.size = size;
		cursor->set_key(cursor, &key);
	}
	return cursor->remove(cursor);
}

int wiredtiger_cursor_reset(WT_CURSOR *cursor) {
	return cursor->reset(cursor);
}

int wiredtiger_cursor_close(WT_CURSOR *cursor) {
	return cursor->close(cursor);
}
*/
import "C"
import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"

	"github.com/FavorLabs/favorX/pkg/shed/driver"
)

type cursorCache struct {
	inuse  uint32
	cursor *cursor
}

func newCursorCache() *cursorCache {
	return &cursorCache{}
}

type cursorOption struct {
	Bulk      bool `key:"-"`
	Raw       bool
	ReadOnce  bool
	Overwrite bool
}

var ErrCursorHasClosed = errors.New("cursor has closed")

func (cc *cursorCache) newCursor(s *session, uri string, opt *cursorOption) (*cursor, error) {
	optStr := structToList(*opt, false)

	for {
		if atomic.LoadUint32(&cc.inuse) == 2 {
			return nil, ErrCursorHasClosed
		}
		if !atomic.CompareAndSwapUint32(&cc.inuse, 0, 1) {
			runtime.Gosched()
			continue
		}

		if cc.cursor != nil {
			// reconfigure
			cfg := C.CString(optStr)

			result := int(C.wiredtiger_cursor_reconfigure(cc.cursor.impl, cfg))
			if checkError(result) {
				return nil, NewError(result, cc.cursor.s)
			}

			C.free(unsafe.Pointer(cfg))

			return cc.cursor, nil
		}

		c, err := openCursor(s, uri, optStr)
		if err != nil {
			return nil, err
		}

		cc.cursor = c

		atomic.StoreUint32(&cc.inuse, 1)

		return c, nil
	}
}

func (cc *cursorCache) releaseCursor(c *cursor) {
	err := c.reset()
	if err != nil {
		logger.Errorf("reset cursor: %v", err)
	}

	atomic.StoreUint32(&cc.inuse, 0)
}

func (cc *cursorCache) closeAll(_ string) error {
	if cc.cursor != nil {
		err := cc.cursor.close()
		if err != nil {
			return err
		}
		cc.cursor = nil
	}

	atomic.StoreUint32(&cc.inuse, 2)

	return nil
}

type cursor struct {
	s    *session
	impl *C.WT_CURSOR
	typ  dataType
	uri  string
	kf   string // key format
	vf   string // value format
	exit bool
}

func openCursor(s *session, uri, config string) (*cursor, error) {
	var wc *C.WT_CURSOR
	var configStr *C.char = nil

	uriStr := C.CString(uri)
	defer C.free(unsafe.Pointer(uriStr))

	if len(config) > 0 {
		configStr = C.CString(config)
		defer C.free(unsafe.Pointer(configStr))
	}

	result := int(C.wiredtiger_session_open_cursor(s.impl, uriStr, nil, configStr, &wc))
	if checkError(result) {
		return nil, NewError(result, s)
	}

	c := &cursor{
		s:    s,
		impl: wc,
		uri:  uri,
		kf:   C.GoString(wc.key_format),
		vf:   C.GoString(wc.value_format),
	}

	return c, nil
}

func (c *cursor) insert(key, value []byte) error {
	if len(key) == 0 {
		return ErrInvalidArgument
	}

	keyData := C.CBytes(key) // unsafe.Pointer
	keySize := C.size_t(len(key))

	defer C.free(keyData)

	if len(value) == 0 {
		return ErrInvalidArgument
	}

	valueData := C.CBytes(value) // unsafe.Pointer
	valueSize := C.size_t(len(value))

	defer C.free(valueData)

	result := int(C.wiredtiger_cursor_insert(c.impl, keyData, keySize, valueData, valueSize))
	if checkError(result) {
		return NewError(result, c.s)
	}
	return nil
}

func (c *cursor) update(key, value []byte) error {
	if len(key) == 0 {
		return ErrInvalidArgument
	}

	keyData := C.CBytes(key) // unsafe.Pointer
	keySize := C.size_t(len(key))

	defer C.free(keyData)

	if len(value) == 0 {
		return ErrInvalidArgument
	}

	valueData := C.CBytes(value) // unsafe.Pointer
	valueSize := C.size_t(len(value))

	defer C.free(valueData)

	result := int(C.wiredtiger_cursor_update(c.impl, keyData, keySize, valueData, valueSize))
	if checkError(result) {
		return NewError(result, c.s)
	}
	return nil
}

func (c *cursor) find(key []byte) (*resultCursor, error) {
	if len(key) == 0 {
		return nil, ErrNotFound
	}

	data := C.CBytes(key) // unsafe.Pointer
	size := C.size_t(len(key))

	defer C.free(data)

	result := int(C.wiredtiger_cursor_search(c.impl, data, size))
	if checkError(result) {
		return nil, NewError(result)
	}

	return &resultCursor{cursor: c, k: key}, nil
}

func (c *cursor) search(key []byte) (s *searchCursor, err error) {
	var compare C.int
	var data unsafe.Pointer
	var size C.size_t = 0

	if len(key) == 0 {
		key = []byte{0}
	}

	data = C.CBytes(key)
	size = C.size_t(len(key))

	defer C.free(data)

	result := int(C.wiredtiger_cursor_search_near(c.impl, data, size, &compare))
	if checkError(result) {
		return nil, NewError(result)
	}

	s = &searchCursor{
		k:      key,
		cursor: c,
		match:  int(compare) == 0,
		direct: int(compare),
	}

	return
}

func (c *cursor) remove(key []byte) error {
	if len(key) == 0 {
		return ErrInvalidArgument
	}

	data := C.CBytes(key) // unsafe.Pointer
	size := C.size_t(len(key))

	defer C.free(data)

	result := int(C.wiredtiger_cursor_remove(c.impl, data, size))
	if checkError(result) {
		return NewError(result, c.s)
	}

	return nil
}

// key: Must be called by locked-state
func (c *cursor) key() ([]byte, error) {
	var item C.WT_ITEM

	result := int(C.wiredtiger_cursor_get_key(c.impl, &item))
	if checkError(result) {
		return nil, NewError(result, c.s)
	}

	return C.GoBytes(item.data, C.int(item.size)), nil
}

// value: Must be called by locked-state
func (c *cursor) value() ([]byte, error) {
	var item C.WT_ITEM

	result := int(C.wiredtiger_cursor_get_value(c.impl, &item))
	if checkError(result) {
		return nil, NewError(result, c.s)
	}

	return C.GoBytes(item.data, C.int(item.size)), nil
}

func (c *cursor) reset() error {
	result := int(C.wiredtiger_cursor_reset(c.impl))
	if checkError(result) {
		return NewError(result, c.s)
	}

	return nil
}

func (c *cursor) close() error {
	if !c.exit {
		c.exit = true
		result := int(C.wiredtiger_cursor_close(c.impl))
		if checkError(result) {
			return NewError(result, c.s)
		}
	}
	return nil
}

type resultCursor struct {
	*cursor
	k   []byte
	err error
}

func (r *resultCursor) Valid() bool {
	return r.exit
}

func (r *resultCursor) Next() bool {
	return false
}

func (r *resultCursor) Prev() bool {
	return false
}

func (r *resultCursor) Last() bool {
	return true
}

func (r *resultCursor) Seek(_ driver.Key) bool {
	return false
}

func (r *resultCursor) Key() []byte {
	data, err := r.key()
	if err != nil {
		r.err = fmt.Errorf("wiredtiger: cursor find key %s: %v", r.k, err)

		return nil
	}
	return data
}

func (r *resultCursor) Value() []byte {
	data, err := r.value()
	if err != nil {
		r.err = fmt.Errorf("wiredtiger: cursor find value for key %s: %v", r.k, err)

		return nil
	}
	return data
}

func (r *resultCursor) Error() error {
	return r.err
}

func (r *resultCursor) Close() error {
	return nil
}

type searchCursor struct {
	*cursor
	prefix []byte
	k      []byte
	err    error
	match  bool
	direct int // search near flag
}

func (s *searchCursor) Error() error {
	return s.err
}

func (s *searchCursor) Valid() bool {
	return !s.exit && s.err == nil
}

func (s *searchCursor) Last() bool {
	err := s.reset()
	if err != nil {
		s.err = err
		return false
	}

	return s.Prev()
}

func (s *searchCursor) Next() bool {
	if s.match {
		s.match = false
	} else {
		result := int(C.wiredtiger_cursor_next(s.impl))
		if checkError(result) {
			err := NewError(result)
			if !IsNotFound(err) {
				s.err = fmt.Errorf("wiredtiger: cursor walk forward for key %s: %v", s.k, err)
			}

			return false
		}
	}

	return true
}

func (s *searchCursor) Prev() bool {
	if s.match {
		s.match = false
	} else {
		result := int(C.wiredtiger_cursor_prev(s.impl))
		if checkError(result) {
			err := NewError(result)
			if !IsNotFound(err) {
				s.err = fmt.Errorf("wiredtiger: cursor walk backward for key %s: %v", s.k, err)
			}

			return false
		}
	}

	return true
}

func (s *searchCursor) Seek(key driver.Key) bool {
	// check prefix
	table := strings.SplitN(s.uri, ":", 2)[1]
	obj, k := parseKey(key)
	switch obj.sourceName {
	case stateTableName:
		if table != stateTableName {
			return false
		}
	case fieldTableName:
		if table != fieldTableName {
			return false
		}
	default:
		if !strings.HasPrefix(obj.sourceName, indexTablePrefix) {
			s.err = fmt.Errorf("wiredtiger: parse unknown key type: %s", k)
			return false
		}
		if table != obj.sourceName {
			s.err = fmt.Errorf("wiredtiger: expect to find table %s, not %s", table, obj.sourceName)
			return false
		}
	}

	c, err := s.search(k)

	if err != nil {
		s.err = fmt.Errorf("wiredtiger: cursor seek key %s: %v", k, err)
		return false
	}

	s.k = c.k
	s.cursor = c.cursor
	s.direct = c.direct

	return true
}

func (s *searchCursor) Key() []byte {
	s.match = false
	key, err := s.key()
	if err != nil {
		s.err = fmt.Errorf("wiredtiger: cursor search key %s: %v", s.k, err)
	}
	data := make([]byte, len(s.prefix)+len(key))
	copy(data, s.prefix)
	copy(data[len(s.prefix):], key)
	return data
}

func (s *searchCursor) Value() []byte {
	s.match = false
	data, err := s.value()
	if err != nil {
		s.err = fmt.Errorf("wiredtiger: cursor search value for key %s: %v", s.k, err)
	}
	return data
}

func (s *searchCursor) Close() error {
	s.s.cursors.releaseCursor(s.cursor)

	// put session
	s.s.ref.Put(s.s)

	return nil
}

type errorCursor struct {
	err error
}

var InvalidCursor = &errorCursor{}

func (e *errorCursor) Valid() bool {
	return false
}

func (e *errorCursor) Next() bool {
	return false
}

func (e *errorCursor) Prev() bool {
	return false
}

func (e *errorCursor) Last() bool {
	return false
}

func (e *errorCursor) Seek(_ driver.Key) bool {
	return false
}

func (e *errorCursor) Key() []byte {
	return nil
}

func (i *errorCursor) Value() []byte {
	return nil
}

func (e *errorCursor) Error() error {
	if e.err != nil {
		return fmt.Errorf("wiredtiger: cursor error: %s", e.err)
	}

	return nil
}

func (e *errorCursor) Close() error {
	return nil
}
