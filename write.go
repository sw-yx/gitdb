package gitdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
)

func (g *gdb) insert(m Model) error {

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
	}

	return g.queue(m)
}

func (g *gdb) queue(m Model) error {

	dataBlock, err := g.loadBlock(g.queueFilePath(m), m.GetSchema().Name())
	if err != nil {
		return err
	}

	writeErr := g.writeBlock(g.queueFilePath(m), dataBlock)
	if writeErr != nil {
		return writeErr
	}

	return g.updateId(m)
}

func (g *gdb) flushQueue(m Model) error {

	if _, err := os.Stat(g.queueFilePath(m)); err == nil {

		log("flushing queue")
		dataBlock := NewBlock(m.GetSchema().Name())
		err := g.readBlock(g.queueFilePath(m), dataBlock)
		if err != nil {
			return err
		}

		//todo optimize: this will open and close block file to delete each record it flushes
		model := g.config.Factory(m.GetSchema().Name())
		for recordId, record := range dataBlock.records {
			log("Flushing: " + recordId)

			record.Hydrate(model)

			err = g.write(model)
			if err != nil {
				println(err.Error())
				return err
			}
			err = g.delById(recordId, m.GetSchema().Name(), g.queueFilePath(m), false)
			if err != nil {
				return err
			}
		}

		return os.Remove(g.queueFilePath(m))
	}

	log("empty queue :)")

	return nil
}

func (g *gdb) flushDb() error {
	return nil
}

func (g *gdb) write(m Model) error {

	blockFilePath := g.blockFilePath(m)
	commitMsg := "Inserting " + m.Id() + " into " + blockFilePath

	dataBlock, err := g.loadBlock(blockFilePath, m.GetSchema().Name())
	if err != nil {
		return err
	}

	//...append new record to block
	newRecordBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	if _, err := dataBlock.Get(m.Id()); err == nil {
		commitMsg = "Updating " + m.Id() + " in " + blockFilePath
	}

	logTest(commitMsg)

	newRecordStr := string(newRecordBytes)
	dataBlock.Add(m.GetSchema().RecordId(), newRecordStr)

	g.events <- newWriteBeforeEvent("...", blockFilePath)
	writeErr := g.writeBlock(blockFilePath, dataBlock)
	if writeErr != nil {
		return writeErr
	}

	log(fmt.Sprintf("autoCommit: %v", g.autoCommit))

	logTest("sending write event to loop")
	g.events <- newWriteEvent(commitMsg, blockFilePath)
	g.updateIndexes(m.GetSchema().Name(), newRecord(m.Id(), newRecordStr))

	//what is the effect of this on InsertMany?
	return g.updateId(m)
}

func (g *gdb) writeBlock(blockFile string, block *Block) error {

	model := g.getModelFromCache(block.dataset)

	//encrypt data if need be
	if model.ShouldEncrypt() {
		for k, record := range block.records {
			block.Add(k, encrypt(g.config.EncryptionKey, record.data))
		}
	}

	blockBytes, fmtErr := json.MarshalIndent(block.data(), "", "\t")
	if fmtErr != nil {
		return fmtErr
	}

	return ioutil.WriteFile(blockFile, blockBytes, 0744)
}

func (g *gdb) delete(id string) error {
	return g.dodelete(id, false)
}

func (g *gdb) deleteOrFail(id string) error {
	return g.dodelete(id, true)
}

func (g *gdb) dodelete(id string, failNotFound bool) error {

	dataDir, _, _, err := ParseId(id)
	if err != nil {
		return err
	}

	model := g.getModelFromCache(dataDir)

	blockFilePath := g.blockFilePath(model)
	err = g.delById(id, dataDir, blockFilePath, failNotFound)

	if err == nil {
		logTest("sending delete event to loop")
		g.events <- newDeleteEvent("Deleting "+id+" in "+blockFilePath, blockFilePath)
	}

	return err
}

func (g *gdb) delById(id string, dataset string, blockFile string, failIfNotFound bool) error {

	if _, err := os.Stat(blockFile); err != nil {
		if failIfNotFound {
			return errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return nil
	}

	dataBlock := NewBlock(dataset)
	err := g.readBlock(blockFile, dataBlock)
	if err != nil {
		return err
	}

	if err := dataBlock.Delete(id); err != nil {
		if failIfNotFound {
			return errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return nil
	}

	//write undeleted records back to block file
	return g.writeBlock(blockFile, dataBlock)
}
