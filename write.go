package gitdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"gopkg.in/mgo.v2/bson"
)

func (g *Gitdb) Insert(m Model) error {

	stamp(m)

	if _, err := os.Stat(g.fullPath(m)); err != nil {
		os.MkdirAll(g.fullPath(m), 0755)
	}

	if !m.Validate() {
		return errors.New("Model is not valid")
	}

	if g.getLock() {
		if err := g.flushQueue(m); err != nil {
			log(err.Error())
		}
		err := g.write(m)
		g.releaseLock()
		return err
	} else {
		return g.queue(m)
	}
}

func (g *Gitdb) InsertMany(models []Model) error {
	//todo polish this up later
	if len(models) > 100 {
		return errors.New("max number of models InsertMany supports is 100")
	}

	tx := g.NewTransaction("InsertMany")
	for _, m := range models {
		tx.AddOperation(func() error { return g.Insert(m) })
	}
	return tx.Commit()
}

func (g *Gitdb) queue(m Model) error {

	dataBlock, err := g.loadBlock(g.queueFilePath(m), m.GetDataFormat())
	if err != nil {
		return err
	}

	writeErr := g.writeBlock(g.queueFilePath(m), dataBlock, m.GetDataFormat(), m.ShouldEncrypt())
	if writeErr != nil {
		return writeErr
	}

	return g.updateId(m)
}

func (g *Gitdb) flushQueue(m Model) error {

	if _, err := os.Stat(g.queueFilePath(m)); err == nil {

		log("flushing queue")
		dataBlock := Block{}
		err := g.readBlock(g.queueFilePath(m), m.GetDataFormat(), dataBlock)
		if err != nil {
			return err
		}

		//todo optimize: this will open and close file for each write operation
		model := g.config.Factory(m.GetSchema().Name())
		for recordId, record := range dataBlock {
			log("Flushing: " + recordId)

			record.Hydrate(model)

			err = g.write(model)
			if err != nil {
				println(err.Error())
				return err
			}
			err = g.delById(recordId, model.GetDataFormat(), g.queueFilePath(m), false)
			if err != nil {
				return err
			}
		}

		return os.Remove(g.queueFilePath(m))
	}

	log("empty queue :)")

	return nil
}

func (g *Gitdb) flushDb() error {
	return nil
}

func (g *Gitdb) write(m Model) error {

	blockFilePath := g.blockFilePath(m)
	commitMsg := "Inserting " + m.Id() + " into " + blockFilePath

	dataBlock, err := g.loadBlock(blockFilePath, m.GetDataFormat())
	if err != nil {
		return err
	}

	//...append new record to block
	newRecordBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	if _, ok := dataBlock[m.Id()]; ok {
		commitMsg = "Updating " + m.Id() + " in " + blockFilePath
	}

	logTest(commitMsg)

	dataBlock[m.GetSchema().RecordId()] = record(newRecordBytes)

	b := Block{}
	b.Add(m.GetSchema().RecordId(), string(newRecordBytes))

	g.events <- newWriteBeforeEvent("...", blockFilePath)
	writeErr := g.writeBlock(blockFilePath, dataBlock, m.GetDataFormat(), m.ShouldEncrypt())
	if writeErr != nil {
		return writeErr
	}

	log(fmt.Sprintf("autoCommit: %v", g.autoCommit))

	logTest("sending write event to loop")
	g.events <- newWriteEvent(commitMsg, blockFilePath)
	g.updateIndexes(b, m.GetSchema().Name())

	//what is the effect of this on InsertMany?
	return g.updateId(m)
}

func (g *Gitdb) writeBlock(blockFile string, data Block, format DataFormat, encryptData bool) error {

	//encrypt data if need be
	if encryptData {
		for k, v := range data {
			data[k] = record(encrypt(g.config.EncryptionKey, string(v)))
		}
	}

	//determine which format we need to write data in
	var blockBytes []byte
	var fmtErr error
	switch format {
	case JSON:
		blockBytes, fmtErr = json.MarshalIndent(data, "", "\t")
		break
	case BSON:
		blockBytes, fmtErr = bson.Marshal(data)
		break
	}

	if fmtErr != nil {
		return fmtErr
	}

	return ioutil.WriteFile(blockFile, blockBytes, 0744)
}

func (g *Gitdb) Delete(id string) error {
	return g.delete(id, false)
}

func (g *Gitdb) DeleteOrFail(id string) error {
	return g.delete(id, true)
}

func (g *Gitdb) delete(id string, failNotFound bool) error {

	dataDir, _, _, err := g.ParseId(id)
	if err != nil {
		return err
	}

	model := g.getModelFromCache(dataDir)

	blockFilePath := g.blockFilePath(model)
	err = g.delById(id, model.GetDataFormat(), blockFilePath, failNotFound)

	if err == nil {
		logTest("sending delete event to loop")
		g.events <- newDeleteEvent("Deleting "+id+" in "+blockFilePath, blockFilePath)
	}

	return err
}

func (g *Gitdb) delById(id string, format DataFormat, blockFile string, failIfNotFound bool) error {

	if _, err := os.Stat(blockFile); err != nil {
		if failIfNotFound {
			return errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return nil
	}

	dataBlock := Block{}
	err := g.readBlock(blockFile, format, dataBlock)
	if err != nil {
		return err
	}

	if err := dataBlock.Delete(id); err != nil {
		if failIfNotFound {
			return errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return nil
	}

	out, err := json.MarshalIndent(dataBlock, "", "\t")
	if err != nil {
		return err
	}

	//write undeleted records back to block file
	return ioutil.WriteFile(blockFile, out, 0744)
}
