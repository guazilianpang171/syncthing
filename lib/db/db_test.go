// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package db

import (
	"bytes"
	"testing"

	"github.com/syncthing/syncthing/lib/db/backend"
	"github.com/syncthing/syncthing/lib/fs"
	"github.com/syncthing/syncthing/lib/protocol"
)

func genBlocks(n int) []protocol.BlockInfo {
	b := make([]protocol.BlockInfo, n)
	for i := range b {
		h := make([]byte, 32)
		for j := range h {
			h[j] = byte(i + j)
		}
		b[i].Size = int32(i)
		b[i].Hash = h
	}
	return b
}

func TestIgnoredFiles(t *testing.T) {
	ldb, err := openJSONS("testdata/v0.14.48-ignoredfiles.db.jsons")
	if err != nil {
		t.Fatal(err)
	}
	db := NewLowlevel(ldb)
	defer db.Close()
	if err := UpdateSchema(db); err != nil {
		t.Fatal(err)
	}

	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)

	// The contents of the database are like this:
	//
	// 	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeBasic, "."), db)
	// 	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
	// 		{ // invalid (ignored) file
	// 			Name:    "foo",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Invalid: true,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1000}}},
	// 		},
	// 		{ // regular file
	// 			Name:    "bar",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
	// 		},
	// 	})
	// 	fs.Update(protocol.DeviceID{42}, []protocol.FileInfo{
	// 		{ // invalid file
	// 			Name:    "baz",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Invalid: true,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1000}}},
	// 		},
	// 		{ // regular file
	// 			Name:    "quux",
	// 			Type:    protocol.FileInfoTypeFile,
	// 			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 42, Value: 1002}}},
	// 		},
	// 	})

	// Local files should have the "ignored" bit in addition to just being
	// generally invalid if we want to look at the simulation of that bit.

	snap := fs.Snapshot()
	defer snap.Release()
	fi, ok := snap.Get(protocol.LocalDeviceID, "foo")
	if !ok {
		t.Fatal("foo should exist")
	}
	if !fi.IsInvalid() {
		t.Error("foo should be invalid")
	}
	if !fi.IsIgnored() {
		t.Error("foo should be ignored")
	}

	fi, ok = snap.Get(protocol.LocalDeviceID, "bar")
	if !ok {
		t.Fatal("bar should exist")
	}
	if fi.IsInvalid() {
		t.Error("bar should not be invalid")
	}
	if fi.IsIgnored() {
		t.Error("bar should not be ignored")
	}

	// Remote files have the invalid bit as usual, and the IsInvalid() method
	// should pick this up too.

	fi, ok = snap.Get(protocol.DeviceID{42}, "baz")
	if !ok {
		t.Fatal("baz should exist")
	}
	if !fi.IsInvalid() {
		t.Error("baz should be invalid")
	}
	if !fi.IsInvalid() {
		t.Error("baz should be invalid")
	}

	fi, ok = snap.Get(protocol.DeviceID{42}, "quux")
	if !ok {
		t.Fatal("quux should exist")
	}
	if fi.IsInvalid() {
		t.Error("quux should not be invalid")
	}
	if fi.IsInvalid() {
		t.Error("quux should not be invalid")
	}
}

const myID = 1

var (
	remoteDevice0, remoteDevice1 protocol.DeviceID
	update0to3Folder             = "UpdateSchema0to3"
	invalid                      = "invalid"
	slashPrefixed                = "/notgood"
	haveUpdate0to3               map[protocol.DeviceID]fileList
)

func init() {
	remoteDevice0, _ = protocol.DeviceIDFromString("AIR6LPZ-7K4PTTV-UXQSMUU-CPQ5YWH-OEDFIIQ-JUG777G-2YQXXR5-YD6AWQR")
	remoteDevice1, _ = protocol.DeviceIDFromString("I6KAH76-66SLLLB-5PFXSOA-UFJCDZC-YAOMLEK-CP2GB32-BV5RQST-3PSROAU")
	haveUpdate0to3 = map[protocol.DeviceID]fileList{
		protocol.LocalDeviceID: {
			protocol.FileInfo{Name: "a", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
			protocol.FileInfo{Name: slashPrefixed, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1000}}}, Blocks: genBlocks(1)},
		},
		remoteDevice0: {
			protocol.FileInfo{Name: "b", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1001}}}, Blocks: genBlocks(2)},
			protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(5), RawInvalid: true},
			protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(7)},
		},
		remoteDevice1: {
			protocol.FileInfo{Name: "c", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1002}}}, Blocks: genBlocks(7)},
			protocol.FileInfo{Name: "d", Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1003}}}, Blocks: genBlocks(5), RawInvalid: true},
			protocol.FileInfo{Name: invalid, Version: protocol.Vector{Counters: []protocol.Counter{{ID: myID, Value: 1004}}}, Blocks: genBlocks(5), RawInvalid: true},
		},
	}
}

func TestUpdate0to3(t *testing.T) {
	ldb, err := openJSONS("testdata/v0.14.45-update0to3.db.jsons")

	if err != nil {
		t.Fatal(err)
	}

	db := NewLowlevel(ldb)
	defer db.Close()
	updater := schemaUpdater{db}

	folder := []byte(update0to3Folder)

	if err := updater.updateSchema0to1(0); err != nil {
		t.Fatal(err)
	}

	trans, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()
	if _, ok, err := trans.getFile(folder, protocol.LocalDeviceID[:], []byte(slashPrefixed)); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("File prefixed by '/' was not removed during transition to schema 1")
	}

	key, err := db.keyer.GenerateGlobalVersionKey(nil, folder, []byte(invalid))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Get(key); err != nil {
		t.Error("Invalid file wasn't added to global list")
	}

	if err := updater.updateSchema1to2(1); err != nil {
		t.Fatal(err)
	}

	found := false
	trans, err = db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()
	_ = trans.withHaveSequence(folder, 0, func(fi FileIntf) bool {
		f := fi.(protocol.FileInfo)
		l.Infoln(f)
		if found {
			t.Error("Unexpected additional file via sequence", f.FileName())
			return true
		}
		if e := haveUpdate0to3[protocol.LocalDeviceID][0]; f.IsEquivalentOptional(e, 0, true, true, 0) {
			found = true
		} else {
			t.Errorf("Wrong file via sequence, got %v, expected %v", f, e)
		}
		return true
	})
	if !found {
		t.Error("Local file wasn't added to sequence bucket", err)
	}

	if err := updater.updateSchema2to3(2); err != nil {
		t.Fatal(err)
	}

	need := map[string]protocol.FileInfo{
		haveUpdate0to3[remoteDevice0][0].Name: haveUpdate0to3[remoteDevice0][0],
		haveUpdate0to3[remoteDevice1][0].Name: haveUpdate0to3[remoteDevice1][0],
		haveUpdate0to3[remoteDevice0][2].Name: haveUpdate0to3[remoteDevice0][2],
	}
	trans, err = db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.Release()
	_ = trans.withNeed(folder, protocol.LocalDeviceID[:], false, func(fi FileIntf) bool {
		e, ok := need[fi.FileName()]
		if !ok {
			t.Error("Got unexpected needed file:", fi.FileName())
		}
		f := fi.(protocol.FileInfo)
		delete(need, f.Name)
		if !f.IsEquivalentOptional(e, 0, true, true, 0) {
			t.Errorf("Wrong needed file, got %v, expected %v", f, e)
		}
		return true
	})
	for n := range need {
		t.Errorf(`Missing needed file "%v"`, n)
	}
}

// TestRepairSequence checks that a few hand-crafted messed-up sequence entries get fixed.
func TestRepairSequence(t *testing.T) {
	db := NewLowlevel(backend.OpenMemory())
	defer db.Close()

	folderStr := "test"
	folder := []byte(folderStr)
	id := protocol.LocalDeviceID
	short := protocol.LocalDeviceID.Short()

	files := []protocol.FileInfo{
		{Name: "fine"},
		{Name: "duplicate"},
		{Name: "missing"},
		{Name: "overwriting"},
		{Name: "inconsistent"},
	}
	for i, f := range files {
		files[i].Version = f.Version.Update(short)
	}

	trans, err := db.newReadWriteTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer trans.close()

	addFile := func(f protocol.FileInfo, seq int64) {
		dk, err := trans.keyer.GenerateDeviceFileKey(nil, folder, id[:], []byte(f.Name))
		if err != nil {
			t.Fatal(err)
		}
		if err := trans.putFile(dk, f); err != nil {
			t.Fatal(err)
		}
		sk, err := trans.keyer.GenerateSequenceKey(nil, folder, seq)
		if err != nil {
			t.Fatal(err)
		}
		if err := trans.Put(sk, dk); err != nil {
			t.Fatal(err)
		}
	}

	// Plain normal entry
	var seq int64 = 1
	files[0].Sequence = 1
	addFile(files[0], seq)

	// Second entry once updated with original sequence still in place
	f := files[1]
	f.Sequence = int64(len(files) + 1)
	addFile(f, f.Sequence)
	// Original sequence entry
	seq++
	sk, err := trans.keyer.GenerateSequenceKey(nil, folder, seq)
	if err != nil {
		t.Fatal(err)
	}
	dk, err := trans.keyer.GenerateDeviceFileKey(nil, folder, id[:], []byte(f.Name))
	if err != nil {
		t.Fatal(err)
	}
	if err := trans.Put(sk, dk); err != nil {
		t.Fatal(err)
	}

	// File later overwritten thus missing sequence entry
	seq++
	files[2].Sequence = seq
	addFile(files[2], seq)

	// File overwriting previous sequence entry (no seq bump)
	seq++
	files[3].Sequence = seq
	addFile(files[3], seq)

	// Inconistent file
	seq++
	files[4].Sequence = 101
	addFile(files[4], seq)

	// And a sequence entry pointing at nothing because why not
	sk, err = trans.keyer.GenerateSequenceKey(nil, folder, 100001)
	if err != nil {
		t.Fatal(err)
	}
	dk, err = trans.keyer.GenerateDeviceFileKey(nil, folder, id[:], []byte("nonexisting"))
	if err != nil {
		t.Fatal(err)
	}
	if err := trans.Put(sk, dk); err != nil {
		t.Fatal(err)
	}

	if err := trans.Commit(); err != nil {
		t.Fatal(err)
	}

	// Loading the metadata for the first time means a "re"calculation happens,
	// along which the sequences get repaired too.
	db.gcMut.RLock()
	_ = loadMetadataTracker(db, folderStr)
	db.gcMut.RUnlock()
	if err != nil {
		t.Fatal(err)
	}

	// Check the db
	ro, err := db.newReadOnlyTransaction()
	if err != nil {
		t.Fatal(err)
	}
	defer ro.close()

	it, err := ro.NewPrefixIterator([]byte{KeyTypeDevice})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Release()
	for it.Next() {
		fi, err := ro.unmarshalTrunc(it.Value(), true)
		if err != nil {
			t.Fatal(err)
		}
		if sk, err = ro.keyer.GenerateSequenceKey(sk, folder, fi.SequenceNo()); err != nil {
			t.Fatal(err)
		}
		dk, err := ro.Get(sk)
		if backend.IsNotFound(err) {
			t.Error("Missing sequence entry for", fi.FileName())
		} else if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(it.Key(), dk) {
			t.Errorf("Wrong key for %v, expected %s, got %s", f.FileName(), it.Key(), dk)
		}
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	it.Release()

	it, err = ro.NewPrefixIterator([]byte{KeyTypeSequence})
	if err != nil {
		t.Fatal(err)
	}
	defer it.Release()
	for it.Next() {
		fi, ok, err := ro.getFileTrunc(it.Value(), true)
		if err != nil {
			t.Fatal(err)
		}
		seq := ro.keyer.SequenceFromSequenceKey(it.Key())
		if !ok {
			t.Errorf("Sequence entry %v points at nothing", seq)
		} else if fi.SequenceNo() != seq {
			t.Errorf("Inconsistent sequence entry for %v: %v != %v", fi.FileName(), fi.SequenceNo(), seq)
		}
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	it.Release()
}

func TestDowngrade(t *testing.T) {
	db := NewLowlevel(backend.OpenMemory())
	defer db.Close()
	// sets the min version etc
	if err := UpdateSchema(db); err != nil {
		t.Fatal(err)
	}

	// Bump the database version to something newer than we actually support
	miscDB := NewMiscDataNamespace(db)
	if err := miscDB.PutInt64("dbVersion", dbVersion+1); err != nil {
		t.Fatal(err)
	}
	l.Infoln(dbVersion)

	// Pretend we just opened the DB and attempt to update it again
	err := UpdateSchema(db)

	if err, ok := err.(databaseDowngradeError); !ok {
		t.Fatal("Expected error due to database downgrade, got", err)
	} else if err.minSyncthingVersion != dbMinSyncthingVersion {
		t.Fatalf("Error has %v as min Syncthing version, expected %v", err.minSyncthingVersion, dbMinSyncthingVersion)
	}
}

func TestCheckGlobals(t *testing.T) {
	db := NewLowlevel(backend.OpenMemory())
	defer db.Close()

	fs := NewFileSet("test", fs.NewFilesystem(fs.FilesystemTypeFake, ""), db)

	// Add any file
	name := "foo"
	fs.Update(protocol.LocalDeviceID, []protocol.FileInfo{
		{
			Name:    name,
			Type:    protocol.FileInfoTypeFile,
			Version: protocol.Vector{Counters: []protocol.Counter{{ID: 1, Value: 1001}}},
		},
	})

	// Remove just the file entry
	if err := db.dropPrefix([]byte{KeyTypeDevice}); err != nil {
		t.Fatal(err)
	}

	// Clean up global entry of the now missing file
	if err := db.checkGlobals([]byte(fs.folder), fs.meta); err != nil {
		t.Fatal(err)
	}

	// Check that the global entry is gone
	gk, err := db.keyer.GenerateGlobalVersionKey(nil, []byte(fs.folder), []byte(name))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Get(gk)
	if !backend.IsNotFound(err) {
		t.Error("Expected key missing error, got", err)
	}
}
