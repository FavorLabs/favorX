package storagefiles

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall"
)

type DiskManager struct {
	path      string
	size      uint64
	Redundant uint64
}

func NewDiskManager(cfg Config) (*DiskManager, error) {
	dm := &DiskManager{
		path:      cfg.DataDir,
		size:      cfg.Capacity,
		Redundant: cfg.Redundant,
	}

	avail := dm.realAvail()
	if avail < dm.Redundant {
		return nil, fmt.Errorf("disk space is less than %dMB", dm.Redundant/1024/1024)
	}

	return dm, nil
}

func (dm *DiskManager) realAvail() uint64 {
	var stat syscall.Statfs_t

	_ = syscall.Statfs(dm.path, &stat)

	return stat.Bavail * uint64(stat.Bsize)
}

func (dm *DiskManager) LogicAvail() (uint64, error) {
	avail := dm.realAvail()
	usedSize, err := dm.usedSize()
	if err != nil {
		return 0, err
	}

	if avail > dm.size-usedSize {
		return dm.size - usedSize, nil
	} else {
		return avail, nil
	}

}

func (dm *DiskManager) usedSize() (uint64, error) {
	size := int64(0)
	err := filepath.Walk(dm.path, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return uint64(size), nil
}
