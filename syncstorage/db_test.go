package syncstorage

import (
	"io/ioutil"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func getTestDB() (*DB, error) {
	f, err := ioutil.TempFile("", "syncstorage-test-")
	if err != nil {
		return nil, err
	}

	path := f.Name()
	db, err := NewDB(path)

	if err != nil {
		return nil, err
	}

	return db, nil
}

func TestNewDB(t *testing.T) {
	_, err := getTestDB()
	assert.NoError(t, err)

}

// TestStaticCollectionId ensures common collection
// names are map to standard id numbers. It should also
// save database looks ups for these as they are
// baked in
func TestStaticCollectionId(t *testing.T) {
	assert := assert.New(t)
	db, err := getTestDB()
	if !assert.NoError(err) {
		return
	}

	// make sure static collection ids match names
	commonCols := map[int]string{
		1: "clients", 2: "crypto", 3: "forms", 4: "history",
		5: "keys", 6: "meta", 7: "bookmarks", 8: "prefs",
		9: "tabs", 10: "passwords", 11: "addons",
	}

	// ensure DB actually has predefined common collections
	{
		rows, err := db.db.Query("SELECT Id, Name FROM Collections")
		if !assert.NoError(err) {
			return
		}

		results := make(map[int]string)

		for rows.Next() {
			var id int
			var name string
			if err := rows.Scan(&id, &name); !assert.NoError(err) {
				return
			}
			results[id] = name
		}
		rows.Close()

		for id, name := range commonCols {
			n, ok := results[id]
			assert.True(ok, id) // make sure it exists
			assert.Equal(name, n)
		}
	}

	// test that GetCollectionId returns the correct Ids
	// for the common collections
	{
		for id, name := range commonCols {
			checkid, err := db.GetCollectionId(name)
			if !assert.NoError(err, name) {
				return
			}

			if !assert.Equal(checkid, id, name) {
				return
			}
		}
	}

	// make sure custom collections start at Id: 100
	{
		id, err := db.CreateCollection("col1")
		if !assert.NoError(err) {
			return
		}

		// make sure new collection start at 100
		assert.Equal(id, 100)
	}
}

func TestBsoExists(t *testing.T) {
	assert := assert.New(t)

	db, _ := getTestDB()

	tx, err := db.db.Begin()
	assert.NoError(err)
	found, err := db.bsoExists(tx, 1, "nope")
	assert.False(found)
	assert.NoError(err)
	assert.NoError(tx.Rollback())

	// insert a new BSO and test if a
	// true result comes back
	tx, err = db.db.Begin()
	assert.NoError(err)

	cId := 1
	bId := "testBSO"
	modified := Now()
	payload := "payload"
	sortIndex := 1
	ttl := 1000

	assert.NoError(db.insertBSO(tx, cId, bId, modified, payload, sortIndex, ttl))

	found, err = db.bsoExists(tx, cId, bId)
	assert.NoError(err)
	assert.True(found)
}

func TestUpdateBSOReturnsExpectedError(t *testing.T) {
	db, _ := getTestDB()

	tx, _ := db.db.Begin()
	defer tx.Rollback()

	cId := 1
	bId := "testBSO"

	err := db.updateBSO(tx, cId, bId, Now(), nil, nil, nil)
	assert.Equal(t, ErrNothingToDo, err)
}

func TestPrivateUpdateBSOSuccessfullyUpdatesSingleValues(t *testing.T) {

	assert := assert.New(t)
	db, _ := getTestDB()

	tx, _ := db.db.Begin()

	cId := 1
	bId := "testBSO"
	modified := 0
	payload := "initial value"
	sortIndex := 1
	ttl := 3600 * 1000

	var err error

	assert.NoError(db.insertBSO(tx, cId, bId, modified, payload, sortIndex, ttl))

	payload = "Updated payload"
	modified = Now()
	err = db.updateBSO(tx, cId, bId, modified, &payload, nil, nil)
	if !assert.NoError(err) {
		return
	}

	bso, err := db.getBSO(tx, cId, bId)
	if !assert.NoError(err) {
		return
	}

	assert.True((bso.Modified == modified || bso.Payload == payload || bso.SortIndex == sortIndex || bso.TTL == modified+ttl))

	sortIndex = 2
	modified = Now()
	err = db.updateBSO(tx, cId, bId, modified, nil, &sortIndex, nil)

	bso, err = db.getBSO(tx, cId, bId)
	if !assert.NoError(err) || !assert.NotNil(bso) {
		return
	}

	assert.True(bso.Modified == modified || bso.Payload == payload || bso.SortIndex == sortIndex || bso.TTL == modified+ttl)

	modified = Now()
	err = db.updateBSO(tx, cId, bId, modified, nil, nil, &ttl)
	if !assert.NoError(err) {
		return
	}

	bso, err = db.getBSO(tx, cId, bId)
	if !assert.NoError(err) || !assert.NotNil(bso) {
		return
	}

	assert.True(bso.Modified == modified || bso.Payload == payload || bso.SortIndex == sortIndex || bso.TTL == ttl+modified)
}

func TestPrivateUpdateBSOModifiedNotChangedOnTTLTouch(t *testing.T) {
	assert := assert.New(t)

	db, _ := getTestDB()

	tx, _ := db.db.Begin()

	cId := 1
	bId := "testBSO"
	payload := "hello"
	sortIndex := 1
	ttl := 10
	modified := 3

	err := db.insertBSO(tx, cId, bId, modified, payload, sortIndex, ttl)
	if !assert.NoError(err) {
		return
	}

	ttl = 15
	updateModified := 5
	err = db.updateBSO(tx, cId, bId, updateModified, nil, nil, &ttl)
	if !assert.NoError(err) {
		return
	}

	bso, err := db.getBSO(tx, cId, bId)
	if !assert.NoError(err) || !assert.NotNil(bso) {
		return
	}
	assert.Equal(ttl+updateModified, bso.TTL)
	assert.Equal(modified, bso.Modified)
}

func TestPrivatePutBSOInsertsWithMissingValues(t *testing.T) {
}

func TestPrivatePutBSOUpdates(t *testing.T) {
	assert := assert.New(t)

	db, _ := getTestDB()

	tx, _ := db.db.Begin()
	defer tx.Rollback()

	modified := Now()
	if err := db.putBSO(tx, 1, "1", modified, String("initial"), nil, nil); err != nil {
		t.Error(err)
	}

	payload := String("Updated")
	sortIndex := Int(100)
	newModified := modified + 1000
	err := db.putBSO(tx, 1, "1", newModified, payload, sortIndex, nil)
	if !assert.NoError(err) {
		return
	}
	bso, err := db.getBSO(tx, 1, "1")

	assert.NoError(err)
	assert.NotNil(bso)

	assert.Equal(*payload, bso.Payload)
	assert.Equal(*sortIndex, bso.SortIndex)
	assert.Equal(newModified, bso.Modified)
}

func TestPrivateGetBSOsLimitOffset(t *testing.T) {

	assert := assert.New(t)

	db, _ := getTestDB()

	tx, _ := db.db.Begin()
	defer tx.Rollback()

	cId := 1

	// put in enough records to test offset
	totalRecords := 12
	for i := 0; i < totalRecords; i++ {
		id := strconv.Itoa(i)
		payload := "payload-" + id
		sortIndex := i
		modified := Now()
		if err := db.insertBSO(tx, cId, id, modified, payload, sortIndex, DEFAULT_BSO_TTL); err != nil {
			t.Fatal("Error inserting BSO #", i, ":", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	newer := 0
	limit := 5
	offset := 0

	// make sure invalid values don't work for limit and offset
	_, err := db.getBSOs(tx, cId, nil, newer, SORT_INDEX, -1, offset)
	assert.Equal(ErrInvalidLimit, err)
	_, err = db.getBSOs(tx, cId, nil, newer, SORT_INDEX, limit, -1)
	assert.Equal(ErrInvalidOffset, err)

	results, err := db.getBSOs(tx, cId, nil, newer, SORT_NEWEST, limit, offset)
	assert.NoError(err)

	if assert.NotNil(results) {
		assert.Equal(5, len(results.BSOs), "Expected 5 results")
		assert.Equal(totalRecords, results.Total, "Expected %d bsos to be found", totalRecords)
		assert.True(results.More)
		assert.Equal(5, results.Offset, "Expected next offset to be 5")

		// make sure we get the right BSOs
		assert.Equal("11", results.BSOs[0].Id, "Expected BSO w/ Id = 11")
		assert.Equal("7", results.BSOs[4].Id, "Expected BSO w/ Id = 7")
	}

	results2, err := db.getBSOs(tx, cId, nil, newer, SORT_INDEX, limit, results.Offset)
	assert.NoError(err)
	if assert.NotNil(results2) {
		assert.Equal(5, len(results2.BSOs), "Expected 5 results")
		assert.Equal(totalRecords, results.Total, "Expected %d bsos to be found", totalRecords)
		assert.True(results2.More)
		assert.Equal(10, results2.Offset, "Expected next offset to be 10")

		// make sure we get the right BSOs
		assert.Equal("6", results2.BSOs[0].Id, "Expected BSO w/ Id = 5")
		assert.Equal("2", results2.BSOs[4].Id, "Expected BSO w/ Id = 9")
	}

	results3, err := db.getBSOs(tx, cId, nil, newer, SORT_INDEX, limit, results2.Offset)
	assert.NoError(err)
	if assert.NotNil(results3) {
		assert.Equal(2, len(results3.BSOs), "Expected 2 results")
		assert.Equal(totalRecords, results.Total, "Expected %d bsos to be found", totalRecords)
		assert.False(results3.More)

		// make sure we get the right BSOs
		assert.Equal("1", results3.BSOs[0].Id, "Expected BSO w/ Id = 1")
		assert.Equal("0", results3.BSOs[1].Id, "Expected BSO w/ Id = 0")
	}

}

func TestPrivateGetBSOsNewer(t *testing.T) {

	assert := assert.New(t)

	db, _ := getTestDB()

	tx, _ := db.db.Begin()
	defer tx.Rollback()

	// put in enough records to test Newer
	cId := 1

	modified := Now()

	_, err := db.getBSOs(tx, cId, nil, -1, SORT_NONE, 10, 0)
	assert.Equal(ErrInvalidNewer, err)

	assert.Nil(db.insertBSO(tx, cId, "b2", modified-2, "a", 1, DEFAULT_BSO_TTL))
	assert.Nil(db.insertBSO(tx, cId, "b1", modified-1, "a", 1, DEFAULT_BSO_TTL))
	assert.Nil(db.insertBSO(tx, cId, "b0", modified, "a", 1, DEFAULT_BSO_TTL))

	results, err := db.getBSOs(tx, cId, nil, modified-3, SORT_NEWEST, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(3, results.Total)
		assert.Equal(3, len(results.BSOs))
		assert.Equal("b0", results.BSOs[0].Id)
		assert.Equal("b1", results.BSOs[1].Id)
		assert.Equal("b2", results.BSOs[2].Id)
	}

	results, err = db.getBSOs(tx, cId, nil, modified-2, SORT_NEWEST, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(2, results.Total)
		assert.Equal("b0", results.BSOs[0].Id)
		assert.Equal("b1", results.BSOs[1].Id)
	}

	results, err = db.getBSOs(tx, cId, nil, modified-1, SORT_NEWEST, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(1, results.Total)
		assert.Equal("b0", results.BSOs[0].Id)
	}

	results, err = db.getBSOs(tx, cId, nil, modified, SORT_NEWEST, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(0, results.Total)
	}

}

func TestPrivateGetBSOsSort(t *testing.T) {

	assert := assert.New(t)

	db, _ := getTestDB()

	tx, _ := db.db.Begin()
	defer tx.Rollback()

	// put in enough records to test Newer
	cId := 1

	modified := Now()

	_, err := db.getBSOs(tx, cId, nil, -1, SORT_NONE, 10, 0)
	assert.Equal(ErrInvalidNewer, err)

	assert.Nil(db.insertBSO(tx, cId, "b2", modified-2, "a", 2, DEFAULT_BSO_TTL))
	assert.Nil(db.insertBSO(tx, cId, "b1", modified-1, "a", 0, DEFAULT_BSO_TTL))
	assert.Nil(db.insertBSO(tx, cId, "b0", modified, "a", 1, DEFAULT_BSO_TTL))

	results, err := db.getBSOs(tx, cId, nil, 0, SORT_NEWEST, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(3, len(results.BSOs))
		assert.Equal("b0", results.BSOs[0].Id)
		assert.Equal("b1", results.BSOs[1].Id)
		assert.Equal("b2", results.BSOs[2].Id)
	}

	results, err = db.getBSOs(tx, cId, nil, 0, SORT_OLDEST, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(3, len(results.BSOs))
		assert.Equal("b2", results.BSOs[0].Id)
		assert.Equal("b1", results.BSOs[1].Id)
		assert.Equal("b0", results.BSOs[2].Id)
	}

	results, err = db.getBSOs(tx, cId, nil, 0, SORT_INDEX, 10, 0)
	assert.NoError(err)
	if assert.NotNil(results) {
		assert.Equal(3, len(results.BSOs))
		assert.Equal("b2", results.BSOs[0].Id)
		assert.Equal("b0", results.BSOs[1].Id)
		assert.Equal("b1", results.BSOs[2].Id)
	}
}

func TestLastModified(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiLastModified(db, t)
}

func TestGetCollectionId(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiGetCollectionId(db, t)
}

func TestGetCollectionModified(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiGetCollectionModified(db, t)
}

func TestCreateCollection(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiCreateCollection(db, t)
}

func TestDeleteCollection(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiDeleteCollection(db, t)
}

func TestDeleteEverything(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiDeleteEverything(db, t)
}

func TestTouchCollection(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiTouchCollection(db, t)
}

func TestInfoCollections(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiInfoCollections(db, t)
}

func TestInfoCollectionUsage(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiInfoCollectionUsage(db, t)
}

func TestInfoCollectionCounts(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiInfoCollectionCounts(db, t)
}

func TestPublicPostBSOs(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiPostBSOs(db, t)
}

func TestPublicPutBSO(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiPutBSO(db, t)
}

func TestPublicGetBSO(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiGetBSO(db, t)
}

func TestPublicGetBSOs(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiGetBSOs(db, t)
}
func TestPublicGetBSOModified(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiGetBSOModified(db, t)
}

func TestDeleteBSO(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiDeleteBSO(db, t)
}
func TestDeleteBSOs(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiDeleteBSOs(db, t)
}

func TestPurgeExpired(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiPurgeExpired(db, t)
}

func TestUsageStats(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiUsageStats(db, t)
}

func TestOptimize(t *testing.T) {
	t.Parallel()
	db, _ := getTestDB()

	testApiOptimize(db, t)
}
