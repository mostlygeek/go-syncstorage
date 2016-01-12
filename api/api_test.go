package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mostlygeek/go-syncstorage/syncstorage"
	"github.com/stretchr/testify/assert"
)

var (
	collectionNames = []string{
		"bookmarks",
		"history",
		"forms",
		"prefs",
		"tabs",
		"passwords",
		"crypto",
		"client",
		"keys",
		"meta",
	}
)

// used for testing that the returned json data is good
type jsResult []jsonBSO
type jsonBSO struct {
	Id        string  `json:"id"`
	Modified  float64 `json:"modified"`
	Payload   string  `json:"payload"`
	SortIndex int     `json:"sortindex"`
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func makeTestDeps() *Dependencies {
	dir, _ := ioutil.TempDir(os.TempDir(), "sync_storage_api_test")
	dispatch, err := syncstorage.NewDispatch(4, dir, syncstorage.TwoLevelPath, 10)
	if err != nil {
		panic(err)
	}

	return &Dependencies{
		Dispatch: dispatch,
	}
}

// testRequest helps remove some boilerplate
func testRequest(method, urlStr string, body io.Reader, deps *Dependencies) *httptest.ResponseRecorder {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		panic(err)
	}

	return testSendRequest(req, deps)
}

func testSendRequest(req *http.Request, deps *Dependencies) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	if deps == nil {
		deps = makeTestDeps()
	}
	router := NewRouter(deps)
	router.ServeHTTP(w, req)
	return w
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()
	w := testRequest("GET", "http://test/__heartbeat__", nil, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "OK", w.Body.String())
}

func TestEchoUid(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	w := testRequest("GET", "http://test/1.5/123456/echo-uid", nil, nil)

	assert.Equal(http.StatusOK, w.Code)
	assert.Equal("123456", w.Body.String())

	// test that a non-numeric regex fails
	for _, uid := range []string{"a123", "123a", "abcd"} {
		w := testRequest("GET", "http://test/1.5/"+uid+"/echo-uid", nil, nil)
		assert.Equal(http.StatusNotFound, w.Code, "\"%s\" should not have matched route", uid)
	}

}

func TestInfoCollections(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()

	uid := "123456"
	modified := syncstorage.Now()
	expected := map[string]int{
		"bookmarks": modified,
		"history":   modified + 1,
		"forms":     modified + 2,
		"prefs":     modified + 3,
		"tabs":      modified + 4,
		"passwords": modified + 5,
		"crypto":    modified + 6,
		"client":    modified + 7,
		"keys":      modified + 8,
		"meta":      modified + 9,
	}

	for cName, modified := range expected {
		cId, err := deps.Dispatch.GetCollectionId(uid, cName)
		if !assert.NoError(err, "%v", err) {
			return
		}
		err = deps.Dispatch.TouchCollection(uid, cId, modified)
		if !assert.NoError(err, "%v", err) {
			return
		}
	}

	resp := testRequest("GET", "http://test/1.5/"+uid+"/info/collections", nil, deps)

	if !assert.Equal(http.StatusOK, resp.Code) {
		return
	}

	data := resp.Body.Bytes()
	var collections map[string]int
	err := json.Unmarshal(data, &collections)
	if !assert.NoError(err) {
		return
	}

	for cName, expectedTs := range expected {
		ts, ok := collections[cName]
		if assert.True(ok, "expected '%s' collection to be set", cName) {
			assert.Equal(expectedTs, ts)
		}
	}
}

func TestInfoQuota(t *testing.T) { t.Skip("TODO") }
func TestInfoCollectionUsage(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	uid := "12345"
	deps := makeTestDeps()

	sizes := []int{463, 467, 479, 487, 491}

	for _, cName := range collectionNames {
		cId, err := deps.Dispatch.GetCollectionId(uid, cName)
		if !assert.NoError(err, "getting cID: %v", err) {
			return
		}

		for id, size := range sizes {
			payload := strings.Repeat("x", size)
			bId := fmt.Sprintf("bid_%d", id)
			_, err = deps.Dispatch.PutBSO(uid, cId, bId, &payload, nil, nil)
			if !assert.NoError(err, "failed PUT into %s, bid(%s): %v", cName, bId, err) {
				return
			}
		}
	}

	resp := testRequest("GET", "http://test/1.5/"+uid+"/info/collection_usage", nil, deps)
	data := resp.Body.Bytes()

	var collections map[string]int
	err := json.Unmarshal(data, &collections)
	if !assert.NoError(err) {
		return
	}

	var total int
	for _, s := range sizes {
		total += s
	}

	for _, cName := range collectionNames {
		assert.Equal(total, collections[cName])
	}
}

func TestCollectionCounts(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	uid := "12345"
	deps := makeTestDeps()

	expected := make(map[string]int)

	for _, cName := range collectionNames {
		expected[cName] = 5 + rand.Intn(25)
	}

	for cName, numBSOs := range expected {
		cId, err := deps.Dispatch.GetCollectionId(uid, cName)
		if !assert.NoError(err, "getting cID: %v", err) {
			return
		}

		payload := "hello"
		for i := 0; i < numBSOs; i++ {
			bId := fmt.Sprintf("bid%d", i)
			_, err = deps.Dispatch.PutBSO(uid, cId, bId, &payload, nil, nil)
			if !assert.NoError(err, "failed PUT into %s, bid(%s): %v", cName, bId, err) {
				return
			}
		}
	}

	resp := testRequest("GET", "http://test/1.5/"+uid+"/info/collection_counts", nil, deps)
	data := resp.Body.Bytes()

	var collections map[string]int
	err := json.Unmarshal(data, &collections)
	if !assert.NoError(err) {
		return
	}

	for cName, expectedCount := range expected {
		assert.Equal(expectedCount, collections[cName])
	}
}

func TestCollectionGET(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()
	uid := "123456"

	// serverside limiting of the max requests!
	deps.MaxBSOGetLimit = 4

	// MAKE SOME TEST DATA
	cId, _ := deps.Dispatch.GetCollectionId(uid, "bookmarks")
	payload := "some data"
	numBSOsToMake := 5

	for i := 0; i < numBSOsToMake; i++ {
		bId := "bid_" + strconv.Itoa(i)
		sortindex := i
		_, err := deps.Dispatch.PutBSO(uid, cId, bId, &payload, &sortindex, nil)

		// sleep a bit so we get some spacing between bso modified times
		// a doublel digit sleep is required since we're only accurate
		// to the 100th of a millisecond ala sync1.5 api
		time.Sleep(19 * time.Millisecond)
		if !assert.NoError(err) {
			return
		}
	}

	base := "http://test/1.5/" + uid + "/storage/bookmarks"

	// Without `full` just the bsoIds are returned
	{
		resp := testRequest("GET", base+`?sort=newest`, nil, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}
		assert.Equal(`["bid_4","bid_3","bid_2","bid_1"]`, resp.Body.String())
	}

	// test different sort order
	{
		resp := testRequest("GET", base+`?sort=oldest`, nil, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}
		assert.Equal(`["bid_0","bid_1","bid_2","bid_3"]`, resp.Body.String())
	}

	// test full param
	{
		resp := testRequest("GET", base+"?ids=bid_0,bid_1&full=yes&sort=oldest", nil, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		body := resp.Body.Bytes()
		var results jsResult

		if assert.NoError(json.Unmarshal(body, &results), "JSON Decode error") {
			assert.Len(results, 2)
			assert.Equal("bid_0", results[0].Id)
			assert.Equal("bid_1", results[1].Id)

			assert.Equal(payload, results[0].Payload)
			assert.Equal(payload, results[1].Payload)

			assert.Equal(0, results[0].SortIndex)
			assert.Equal(1, results[1].SortIndex)
		}
	}

	// test limit+offset works
	{
		resp := testRequest("GET", base+`?sort=oldest&limit=2`, nil, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}
		assert.Equal(`["bid_0","bid_1"]`, resp.Body.String())

		offset := resp.Header().Get("X-Weave-Next-Offset")
		if !assert.Equal("2", offset) {
			return
		}

		resp2 := testRequest("GET", base+`?sort=oldest&limit=2&offset=`+offset, nil, deps)
		if !assert.Equal(http.StatusOK, resp2.Code) {
			return
		}
		assert.Equal(`["bid_2","bid_3"]`, resp2.Body.String())
	}

	// test automatic max offset. An artificially small MaxBSOGetLimit is defined
	// in deps to make sure this behaves as expected
	{
		// Get everything but make sure the `limit` we have works
		resp := testRequest("GET", base+`?full=yes&sort=newest`, nil, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		body := resp.Body.Bytes()
		var results jsResult

		if assert.NoError(json.Unmarshal(body, &results), "JSON Decode error") {

			assert.Len(results, deps.MaxBSOGetLimit)

			// make sure sort=oldest works
			assert.Equal("bid_4", results[0].Id)
			assert.Equal(payload, results[0].Payload)
			assert.Equal(4, results[0].SortIndex)

			assert.Equal("4", resp.Header().Get("X-Weave-Next-Offset"))
		}
	}

	// Test newer param
	{
		for i := 0; i < numBSOsToMake-1; i++ {
			id := strconv.Itoa(i)
			idexpected := strconv.Itoa(i + 1)

			// Get everything but make sure the `limit` we have works
			resp := testRequest("GET", base+"?full=yes&ids=bid_"+id, nil, deps)
			if !assert.Equal(http.StatusOK, resp.Code) {
				return
			}

			body := resp.Body.Bytes()
			var results jsResult

			if assert.NoError(json.Unmarshal(body, &results), "JSON Decode error") {
				if !assert.Len(results, 1) {
					return
				}

				modified := fmt.Sprintf("%.02f", results[0].Modified)
				url := base + "?full=yes&limit=1&sort=oldest&newer=" + modified

				resp2 := testRequest("GET", url, nil, deps)
				if !assert.Equal(http.StatusOK, resp2.Code) {
					return
				}

				body2 := resp2.Body.Bytes()
				var results2 jsResult
				if assert.NoError(json.Unmarshal(body2, &results2), "JSON Decode error") {
					if !assert.Len(results2, 1) {
						return
					}
					if !assert.Equal("bid_"+idexpected, results2[0].Id, "modified timestamp precision error?") {
						return
					}
				}

			}
		}
	}

	// test non existant collection returns an empty list
	{
		url := "http://test/1.5/" + uid + "/storage/this_is_not_a_real_collection"
		resp := testRequest("GET", url, nil, deps)

		assert.Equal(http.StatusOK, resp.Code)
		assert.Equal("[]", resp.Body.String())
	}

}

func TestCollectionGETValidatesData(t *testing.T) {

	t.Parallel()
	assert := assert.New(t)
	uid := "1234"

	base := "http://test/1.5/" + uid + "/storage/bookmarks?"
	reqs := map[string]int{
		base + "ids=":                        200,
		base + "ids=abd,123,456":             200,
		base + "ids=no\ttabs\tallowed, here": 400,

		base + "newer=":      200,
		base + "newer=1004":  200,
		base + "newer=-1":    400,
		base + "newer=abcde": 400,

		base + "full=ok": 200,
		base + "full=":   200,

		base + "limit=":    200,
		base + "limit=123": 200,
		base + "limit=a":   400,
		base + "limit=0":   400,
		base + "limit=-1":  400,

		base + "offset=":    200,
		base + "offset=0":   200,
		base + "offset=123": 200,
		base + "offset=a":   400,
		base + "offset=-1":  400,

		base + "sort=":        200,
		base + "sort=newest":  200,
		base + "sort=oldest":  200,
		base + "sort=index":   200,
		base + "sort=invalid": 400,
	}

	// reuse a single deps to not a make a bunch
	// of new storage sub-dirs in testing
	deps := makeTestDeps()

	for url, expected := range reqs {
		resp := testRequest("GET", url, nil, deps)
		assert.Equal(expected, resp.Code, fmt.Sprintf("url:%s => %s", url, resp.Body.String()))
	}
}

func TestCollectionPOST(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()

	uid := "123456"

	// Make sure INSERT works first
	body := bytes.NewBufferString(`[
		{"Id":"bso1", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000},
		{"Id":"bso2", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000},
		{"Id":"bso3", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000}
	]`)

	req, _ := http.NewRequest("POST", "http://test/1.5/"+uid+"/storage/bookmarks", body)
	req.Header.Add("Content-Type", "application/json")

	resp := testSendRequest(req, deps)
	assert.Equal(200, resp.Code)

	var results syncstorage.PostResults
	err := json.Unmarshal(resp.Body.Bytes(), &results)
	if !assert.NoError(err) {
		return
	}

	assert.Len(results.Success, 3)
	assert.Len(results.Failed, 0)

	cId, _ := deps.Dispatch.GetCollectionId(uid, "bookmarks")
	for _, bId := range []string{"bso1", "bso2", "bso3"} {
		bso, _ := deps.Dispatch.GetBSO(uid, cId, bId)
		assert.Equal("initial payload", bso.Payload)
		assert.Equal(1, bso.SortIndex)
	}

	// Test that updates work
	body = bytes.NewBufferString(`[
		{"Id":"bso1", "SortIndex": 2},
		{"Id":"bso2", "Payload": "updated payload"},
		{"Id":"bso3", "Payload": "updated payload", "SortIndex":3}
	]`)

	req2, _ := http.NewRequest("POST", "http://test/1.5/"+uid+"/storage/bookmarks", body)
	req2.Header.Add("Content-Type", "application/json")
	resp = testSendRequest(req2, deps)
	assert.Equal(http.StatusOK, resp.Code)

	bso, _ := deps.Dispatch.GetBSO(uid, cId, "bso1")
	assert.Equal(bso.Payload, "initial payload") // stayed the same
	assert.Equal(bso.SortIndex, 2)               // it updated

	bso, _ = deps.Dispatch.GetBSO(uid, cId, "bso2")
	assert.Equal(bso.Payload, "updated payload") // updated
	assert.Equal(bso.SortIndex, 1)               // same

	bso, _ = deps.Dispatch.GetBSO(uid, cId, "bso3")
	assert.Equal(bso.Payload, "updated payload") // updated
	assert.Equal(bso.SortIndex, 3)               // updated
}

func TestCollectionPOSTCreatesCollection(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()

	uid := "123456"

	// Make sure INSERT works first
	body := bytes.NewBufferString(`[
		{"Id":"bso1", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000},
		{"Id":"bso2", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000},
		{"Id":"bso3", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000}
	]`)

	cName := "my_new_collection"

	req, _ := http.NewRequest("POST", "http://test/1.5/"+uid+"/storage/"+cName, body)
	req.Header.Add("Content-Type", "application/json")
	resp := testSendRequest(req, deps)
	if !assert.Equal(http.StatusOK, resp.Code) {
		return
	}

	cId, err := deps.Dispatch.GetCollectionId(uid, cName)
	if !assert.NoError(err) {
		return
	}

	for _, bId := range []string{"bso1", "bso2", "bso3"} {
		b, err := deps.Dispatch.GetBSO(uid, cId, bId)
		assert.NotNil(b)
		assert.NoError(err)
	}
}

func TestCollectionPOSTTooLargePayload(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()

	uid := "123456"
	template := `[{"id":"%s", "payload": "%s", "sortindex": 1, "ttl": 2100000}]`
	bodydata := fmt.Sprintf(template, "test", strings.Repeat("x", MAX_BSO_PAYLOAD_SIZE+1))

	body := bytes.NewBufferString(bodydata)
	req, _ := http.NewRequest("POST", "http://test/1.5/"+uid+"/storage/bookmarks", body)
	req.Header.Add("Content-Type", "application/json")

	res := testSendRequest(req, deps)
	assert.Equal(http.StatusBadRequest, res.Code)
}

func TestCollectionDELETE(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()

	uid := "123456"

	// Make sure INSERT works first
	body := bytes.NewBufferString(`[
		{"Id":"bso1", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000},
		{"Id":"bso2", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000},
		{"Id":"bso3", "Payload": "initial payload", "SortIndex": 1, "TTL": 2100000}
	]`)

	req, _ := http.NewRequest("POST", "http://test/1.5/"+uid+"/storage/my_collection", body)
	req.Header.Add("Content-Type", "application/json")
	resp := testSendRequest(req, deps)
	if !assert.Equal(http.StatusOK, resp.Code) {
		return
	}

	cId, err := deps.Dispatch.GetCollectionId(uid, "my_collection")
	if !assert.NoError(err) {
		return
	}

	resp = testRequest("DELETE", "http://test/1.5/"+uid+"/storage/my_collection", nil, deps)
	assert.Equal(http.StatusOK, resp.Code)
	assert.Equal("ok", resp.Body.String())

	_, err = deps.Dispatch.GetCollectionId(uid, "my_collection")
	assert.Exactly(syncstorage.ErrNotFound, err)

	for _, bId := range []string{"bso1", "bso2", "bso3"} {
		b, err := deps.Dispatch.GetBSO(uid, cId, bId)
		assert.Nil(b)
		assert.Exactly(syncstorage.ErrNotFound, err)
	}
}

func TestBsoGET(t *testing.T) {

	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()
	uid := "123456"
	collection := "bookmarks"
	bsoId := "test"

	var (
		cId int
		err error
	)

	if cId, err = deps.Dispatch.GetCollectionId(uid, collection); !assert.NoError(err) {
		return
	}

	payload := syncstorage.String("test")
	sortIndex := syncstorage.Int(100)
	if _, err = deps.Dispatch.PutBSO(uid, cId, bsoId, payload, sortIndex, nil); !assert.NoError(err) {
		return
	}

	resp := testRequest("GET", "http://test/1.5/"+uid+"/storage/"+collection+"/"+bsoId, nil, deps)
	if !assert.Equal(http.StatusOK, resp.Code) {
		return
	}

	var bso jsonBSO
	if err = json.Unmarshal(resp.Body.Bytes(), &bso); assert.NoError(err) {
		assert.Equal(bsoId, bso.Id)
		assert.Equal(*payload, bso.Payload)
		assert.Equal(*sortIndex, bso.SortIndex)
	}

	// test that we get a 404 from a bso that doesn't exist
	{
		resp := testRequest("GET", "http://test/1.5/"+uid+"/storage/"+collection+"/nope", nil, deps)
		assert.Equal(http.StatusNotFound, resp.Code)
	}

	// test that we get a 404 from a collection that doesn't exist
	{
		resp := testRequest("GET", "http://test/1.5/"+uid+"/storage/nope/nope", nil, deps)
		assert.Equal(http.StatusNotFound, resp.Code)
	}
}

func TestBsoPUT(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	deps := makeTestDeps()
	uid := "123456"
	collection := "bookmarks"
	testNum := 0

	cId, _ := deps.Dispatch.GetCollectionId(uid, collection)

	{
		testNum++
		bsoId := "test" + strconv.Itoa(testNum)
		data := `{"payload":"hello","sortindex":1, "ttl": 1000000}`
		body := bytes.NewBufferString(data)
		resp := testRequest("PUT", "http://test/1.5/"+uid+"/storage/"+collection+"/"+bsoId, body, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		b, err := deps.Dispatch.GetBSO(uid, cId, bsoId)
		assert.NotNil(b)
		assert.Equal("hello", b.Payload)
		assert.Equal(1, b.SortIndex)
		assert.NoError(err)
		assert.NotEqual("", resp.Header().Get("X-Last-Modified"))
	}

	{ // test with fewer params
		testNum++
		bsoId := "test" + strconv.Itoa(testNum)
		data := `{"payload":"hello","sortindex":1}`
		body := bytes.NewBufferString(data)
		resp := testRequest("PUT", "http://test/1.5/"+uid+"/storage/"+collection+"/"+bsoId, body, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		b, err := deps.Dispatch.GetBSO(uid, cId, bsoId)
		assert.NotNil(b)
		assert.NoError(err)
	}

	{ // test with fewer params
		testNum++
		bsoId := "test" + strconv.Itoa(testNum)
		data := `{"payload":"hello", "sortindex":1}`
		body := bytes.NewBufferString(data)
		resp := testRequest("PUT", "http://test/1.5/"+uid+"/storage/"+collection+"/"+bsoId, body, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		b, err := deps.Dispatch.GetBSO(uid, cId, bsoId)
		assert.NotNil(b)
		assert.NoError(err)
	}

	{ // Test Updates
		testNum++
		bsoId := "test" + strconv.Itoa(testNum)
		data := `{"payload":"hello", "sortindex":1}`
		body := bytes.NewBufferString(data)
		resp := testRequest("PUT", "http://test/1.5/"+uid+"/storage/"+collection+"/"+bsoId, body, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		b, err := deps.Dispatch.GetBSO(uid, cId, bsoId)
		assert.NotNil(b)
		assert.NoError(err)

		data = `{"payload":"updated", "sortindex":2}`
		body = bytes.NewBufferString(data)
		resp = testRequest("PUT", "http://test/1.5/"+uid+"/storage/"+collection+"/"+bsoId, body, deps)
		if !assert.Equal(http.StatusOK, resp.Code) {
			return
		}

		b, err = deps.Dispatch.GetBSO(uid, cId, bsoId)
		assert.NotNil(b)
		assert.NoError(err)
		assert.Equal("updated", b.Payload)
		assert.Equal(2, b.SortIndex)
	}

}

func TestBsoDELETE(t *testing.T) { t.Skip("TODO") }
func TestDelete(t *testing.T)    { t.Skip("TODO") }