package gitdb

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
)

func (g *gitdb) Insert(m Model) error {

	stamp(m)

	if _, err := os.Stat(g.fullPath(m)); err != nil {
		err := os.MkdirAll(g.fullPath(m), 0755)
		if err != nil {
			return fmt.Errorf("failed to make dir %s: %w", g.fullPath(m), err)
		}
	}

	if !m.Validate() {
		return errors.New("Model is not valid")
	}

	g.writeMu.Lock()
	defer g.writeMu.Unlock()

	if err := g.flushQueue(); err != nil {
		log(err.Error())
	}
	return g.write(m)
}

func (g *gitdb) InsertMany(models []Model) error {
	//todo polish this up later
	if len(models) > 100 {
		return errors.New("max number of models InsertMany supports is 100")
	}

	tx := g.NewTransaction("InsertMany")
	var model Model
	for _, model = range models {
		//create a new variable to pass to function to avoid
		//passing pointer which will end up inserting the same
		//model multiple times
		m := model
		f := func() error { return g.Insert(m) }
		tx.AddOperation(f)
	}
	return tx.Commit()
}

func (g *gitdb) queue(m Model) error {

	if len(g.writeQueue) == 0 {
		g.writeQueue = map[string]Model{}
	}

	g.writeQueue[m.ID()] = m
	return g.updateID(m)
}

func (g *gitdb) flushQueue() error {

	for id, model := range g.writeQueue {
		log("Flushing: " + id)

		err := g.write(model)
		if err != nil {
			logError(err.Error())
			return err
		}

		delete(g.writeQueue, id)
	}

	return nil
}

func (g *gitdb) write(m Model) error {

	blockFilePath := g.blockFilePath(m)
	schema := m.GetSchema()

	dataBlock, err := g.loadBlock(blockFilePath, schema.Name())
	if err != nil {
		return err
	}

	//...append new record to block
	newRecordBytes, err := json.Marshal(m)
	if err != nil {
		return err
	}

	mID := m.ID()

	//construct a commit message
	commitMsg := "Inserting " + mID + " into " + schema.BlockID()
	if _, err := dataBlock.get(mID); err == nil {
		commitMsg = "Updating " + mID + " in " + schema.BlockID()
	}

	newRecordStr := string(newRecordBytes)
	//encrypt data if need be
	if m.ShouldEncrypt() {
		newRecordStr = encrypt(g.config.EncryptionKey, newRecordStr)
	}

	dataBlock.add(m.GetSchema().RecordID(), newRecordStr)

	g.events <- newWriteBeforeEvent("...", mID)
	if err := g.writeBlock(blockFilePath, dataBlock); err != nil {
		return err
	}

	log(fmt.Sprintf("autoCommit: %v", g.autoCommit))

	g.commit.Add(1)
	g.events <- newWriteEvent(commitMsg, blockFilePath, g.autoCommit)
	logTest("sent write event to loop")
	g.updateIndexes(schema.Name(), newRecord(mID, newRecordStr))

	//block here until write has been committed
	g.waitForCommit()

	//what is the effect of this on InsertMany?
	return g.updateID(m)
}

func (g *gitdb) waitForCommit() {
	if g.autoCommit {
		logTest("waiting for gitdb to commit changes")
		g.commit.Wait()
	}
}

func (g *gitdb) writeBlock(blockFile string, block *gBlock) error {

	blockBytes, fmtErr := json.MarshalIndent(block.data(), "", "\t")
	if fmtErr != nil {
		return fmtErr
	}

	return ioutil.WriteFile(blockFile, blockBytes, 0744)
}

func (g *gitdb) Delete(id string) error {
	return g.dodelete(id, false)
}

func (g *gitdb) DeleteOrFail(id string) error {
	return g.dodelete(id, true)
}

func (g *gitdb) dodelete(id string, failNotFound bool) error {

	dataset, block, _, err := ParseID(id)
	if err != nil {
		return err
	}

	blockFilePath := g.blockFilePath2(dataset, block)
	err = g.delByID(id, dataset, blockFilePath, failNotFound)

	if err == nil {
		logTest("sending delete event to loop")
		g.commit.Add(1)
		g.events <- newDeleteEvent("Deleting "+id+" in "+blockFilePath, blockFilePath, g.autoCommit)
		g.waitForCommit()
	}

	return err
}

func (g *gitdb) delByID(id string, dataset string, blockFile string, failIfNotFound bool) error {

	if _, err := os.Stat(blockFile); err != nil {
		if failIfNotFound {
			return errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return nil
	}

	dataBlock := newBlock(dataset)
	err := g.readBlock(blockFile, dataBlock)
	if err != nil {
		return err
	}

	if err := dataBlock.delete(id); err != nil {
		if failIfNotFound {
			return errors.New("Could not delete [" + id + "]: record does not exist")
		}
		return nil
	}

	//write undeleted records back to block file
	return g.writeBlock(blockFile, dataBlock)
}
