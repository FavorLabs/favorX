package storagefiles

import (
	"encoding/json"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type Database struct {
	db *leveldb.DB
}

type iterFunc func(sessionID string, value *Task) (stop bool, err error)

const (
	TaskPrefix = "task_list"
)

func NewDB(dir string) (*Database, error) {
	db, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		return nil, err
	}
	return &Database{db: db}, nil
}

func (d *Database) SaveTask(task *Task) error {
	b, err := json.Marshal(task)
	if err != nil {
		return err
	}
	sessionID := task.Info.Hash.String() + task.Info.Source.String()
	return d.db.Put([]byte(TaskPrefix+sessionID), b, nil)
}

func (d *Database) DeleteTask(task *Task) error {
	sessionID := task.Info.Hash.String() + task.Info.Source.String()
	return d.db.Delete([]byte(TaskPrefix+sessionID), nil)
}

func (d *Database) GetTask(sessionID string) (*Task, error) {
	get, err := d.db.Get([]byte(TaskPrefix+sessionID), nil)
	if err != nil {
		return nil, err
	}
	return d.convTask(get)
}

func (d *Database) IterateTask(iterFunc iterFunc) error {
	rng := util.BytesPrefix([]byte(TaskPrefix))

	iter := d.db.NewIterator(rng, nil)
	iter.Seek([]byte(TaskPrefix))

	for ; iter.Valid(); iter.Next() {
		key := string(iter.Key())
		v, err := d.convTask(iter.Value())
		if err != nil {
			return err
		}
		stop, err := iterFunc(strings.TrimLeft(key, TaskPrefix), v)
		if err != nil {
			return err
		}
		if stop {
			break
		}
	}
	return nil
}

func (d *Database) convTask(body []byte) (*Task, error) {
	v := make(map[string]interface{})
	err := json.Unmarshal(body, &v)
	if err != nil {
		return nil, err
	}
	v2 := &Task{}
	err = json.Unmarshal(body, v2)
	if err != nil {
		return nil, err
	}
	return v2, nil
}
