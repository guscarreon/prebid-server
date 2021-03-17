package adapterstest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"testing"

	"github.com/jinzhu/copier"
	"github.com/mxmCherry/openrtb"
	"github.com/prebid/prebid-server/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"

	"net/http"
)

// RunJSONBidderTest is a helper method intended to unit test Bidders' adapters.
// It requires that:
//
//   1. Bidders communicate with external servers over HTTP.
//   2. The HTTP request bodies are legal JSON.
//
// Although the project does not require it, we _strongly_ recommend that all Bidders write tests using this.
// Doing so has the following benefits:
//
// 1. This includes some basic tests which confirm that your Bidder is "well-behaved" for all the input samples.
//    For example, "no nil bids are allowed in the returned array".
//    These tests are tedious to write, but help prevent bugs during auctions.
//
// 2. In the future, we plan to auto-generate documentation from the "exemplary" test files.
//    Those docs will teach publishers how to use your Bidder, which should encourage adoption.
//
// To use this method, create *.json files in the following directories:
//
// adapters/{bidder}/{bidder}test/exemplary:
//
//   These show "ideal" BidRequests for your Bidder. If possible, configure your servers to return the same
//   expected responses forever. If your server responds appropriately, our future auto-generated documentation
//   can guarantee Publishers that your adapter works as documented.
//
// adapters/{bidder}/{bidder}test/supplemental:
//
//   Fill this with *.json files which are useful test cases, but are not appropriate for public example docs.
//   For example, a file in this directory might make sure that a mobile-only Bidder returns errors on non-mobile requests.
//
// Then create a test in your adapters/{bidder}/{bidder}_test.go file like so:
//
//   func TestJsonSamples(t *testing.T) {
//     adapterstest.RunJSONBidderTest(t, "{bidder}test", instanceOfYourBidder)
//   }
//
func RunJSONBidderTest(t *testing.T, rootDir string, bidder adapters.Bidder) {
	runTests(t, fmt.Sprintf("%s/exemplary", rootDir), bidder, false, false, false)
	runTests(t, fmt.Sprintf("%s/supplemental", rootDir), bidder, true, false, false)
	runTests(t, fmt.Sprintf("%s/amp", rootDir), bidder, true, true, false)
	runTests(t, fmt.Sprintf("%s/video", rootDir), bidder, false, false, true)
}

// runTests runs all the *.json files in a directory. If allowErrors is false, and one of the test files
// expects errors from the bidder, then the test will fail.
func runTests(t *testing.T, directory string, bidder adapters.Bidder, allowErrors, isAmpTest, isVideoTest bool) {
	if specFiles, err := ioutil.ReadDir(directory); err == nil {
		for _, specFile := range specFiles {
			fileName := fmt.Sprintf("%s/%s", directory, specFile.Name())
			specData, err := loadFile(fileName)
			if err != nil {
				t.Fatalf("Failed to load contents of file %s: %v", fileName, err)
			}

			if !allowErrors && specData.expectsErrors() {
				t.Fatalf("Exemplary spec %s must not expect errors.", fileName)
			}
			runSpec(t, fileName, specData, bidder, isAmpTest, isVideoTest)
		}
	}
}

// LoadFile reads and parses a file as a test case. If something goes wrong, it returns an error.
func loadFile(filename string) (*testSpec, error) {
	specData, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("Failed to read file %s: %v", filename, err)
	}

	var spec testSpec
	if err := json.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("Failed to unmarshal JSON from file: %v", err)
	}

	return &spec, nil
}

// runSpec runs a single test case. It will make sure:
//
//   - That the Bidder does not return nil HTTP requests, bids, or errors inside their lists
//   - That the Bidder's HTTP calls match the spec's expectations.
//   - That the Bidder's Bids match the spec's expectations
//   - That the Bidder's errors match the spec's expectations
//
// More assertions will almost certainly be added in the future, as bugs come up.
func runSpec(t *testing.T, filename string, spec *testSpec, bidder adapters.Bidder, isAmpTest, isVideoTest bool) {
	reqInfo := adapters.ExtraRequestInfo{}
	if isAmpTest {
		// simulates AMP entry point
		reqInfo.PbsEntryPoint = "amp"
	} else if isVideoTest {
		reqInfo.PbsEntryPoint = "video"
	}

	actualReqs := testMakeRequestsImpl(t, filename, spec, bidder, &reqInfo)

	testMakeBidsImpl(t, filename, spec, bidder, actualReqs)
}

func testMakeRequestsImpl(t *testing.T, filename string, spec *testSpec, bidder adapters.Bidder, reqInfo *adapters.ExtraRequestInfo) []*adapters.RequestData {
	t.Helper()

	// Save original bidRequest values to assert no data races occur inside MakeRequests latter
	deepBidReqCopy := openrtb.BidRequest{}
	copier.Copy(&deepBidReqCopy, &spec.BidRequest)
	shallowBidReqCopy := spec.BidRequest

	// Save original []Imp elements to assert no data races occur inside MakeRequests latter
	deepImpCopies := make([]openrtb.Imp, len(spec.BidRequest.Imp))
	shallowImpCopies := make([]openrtb.Imp, len(spec.BidRequest.Imp))
	for i := 0; i < len(spec.BidRequest.Imp); i++ {
		deepImpCopy := openrtb.Imp{}
		copier.Copy(&deepImpCopy, &spec.BidRequest.Imp[i])
		deepImpCopies = append(deepImpCopies, deepImpCopy)

		shallowImpCopy := spec.BidRequest.Imp[i]
		shallowImpCopies = append(shallowImpCopies, shallowImpCopy)
	}

	// Run MakeRequests
	actualReqs, errs := bidder.MakeRequests(&spec.BidRequest, reqInfo)

	// Compare MakeRequests actual output versus expected values found in JSON file
	assertErrorList(t, fmt.Sprintf("%s: MakeRequests", filename), errs, spec.MakeRequestErrors)
	assertMakeRequestsOutput(t, filename, actualReqs, spec.HttpCalls)

	// Assert no data races occur using original bidRequest copies of references and values
	assertNoDataRace(t, &deepBidReqCopy, &shallowBidReqCopy, deepImpCopies, shallowImpCopies)

	return actualReqs
}

func testMakeBidsImpl(t *testing.T, filename string, spec *testSpec, bidder adapters.Bidder, makeRequestsOut []*adapters.RequestData) {
	t.Helper()

	bidResponses := make([]*adapters.BidderResponse, 0)
	var bidsErrs = make([]error, 0, len(spec.MakeBidsErrors))

	// We should have as many bids as number of adapters.RequestData found in MakeRequests output
	for i := 0; i < len(makeRequestsOut); i++ {
		// Run MakeBids with JSON refined spec.HttpCalls info that was asserted to match MakeRequests
		// output inside testMakeRequestsImpl
		thisBidResponse, theseErrs := bidder.MakeBids(&spec.BidRequest, spec.HttpCalls[i].Request.ToRequestData(t), spec.HttpCalls[i].Response.ToResponseData(t))

		bidsErrs = append(bidsErrs, theseErrs...)
		bidResponses = append(bidResponses, thisBidResponse)
	}

	// Assert actual errors thrown by MakeBids implementation versus expected JSON-defined spec.MakeBidsErrors
	assertErrorList(t, fmt.Sprintf("%s: MakeBids", filename), bidsErrs, spec.MakeBidsErrors)

	// Assert MakeBids implementation BidResponses with expected JSON-defined spec.BidResponses[i].Bids
	for i := 0; i < len(spec.BidResponses); i++ {
		assertMakeBidsOutput(t, filename, bidResponses[i].Bids, spec.BidResponses[i].Bids)
	}
}

type testSpec struct {
	BidRequest        openrtb.BidRequest      `json:"mockBidRequest"`
	HttpCalls         []httpCall              `json:"httpCalls"`
	BidResponses      []expectedBidResponse   `json:"expectedBidResponses"`
	MakeRequestErrors []testSpecExpectedError `json:"expectedMakeRequestsErrors"`
	MakeBidsErrors    []testSpecExpectedError `json:"expectedMakeBidsErrors"`
}

type testSpecExpectedError struct {
	Value      string `json:"value"`
	Comparison string `json:"comparison"`
}

func (spec *testSpec) expectsErrors() bool {
	return len(spec.MakeRequestErrors) > 0 || len(spec.MakeBidsErrors) > 0
}

type httpCall struct {
	Request  httpRequest  `json:"expectedRequest"`
	Response httpResponse `json:"mockResponse"`
}

func (req *httpRequest) ToRequestData(t *testing.T) *adapters.RequestData {
	return &adapters.RequestData{
		Method: "POST",
		Uri:    req.Uri,
		Body:   req.Body,
	}
}

type httpRequest struct {
	Body    json.RawMessage `json:"body"`
	Uri     string          `json:"uri"`
	Headers http.Header     `json:"headers"`
}

type httpResponse struct {
	Status  int             `json:"status"`
	Body    json.RawMessage `json:"body"`
	Headers http.Header     `json:"headers"`
}

func (resp *httpResponse) ToResponseData(t *testing.T) *adapters.ResponseData {
	return &adapters.ResponseData{
		StatusCode: resp.Status,
		Body:       resp.Body,
		Headers:    resp.Headers,
	}
}

type expectedBidResponse struct {
	Bids     []expectedBid `json:"bids"`
	Currency string        `json:"currency"`
}

type expectedBid struct {
	Bid  json.RawMessage `json:"bid"`
	Type string          `json:"type"`
}

// ---------------------------------------
// Lots of ugly, repetitive code below here.
//
// reflect.DeepEquals doesn't work because each OpenRTB field has an `ext []byte`, but we really care if those are JSON-equal
//
// Marshalling the structs and then using a JSON-diff library isn't great either, since

// assertMakeRequestsOutput compares the actual http requests to the expected ones.
func assertMakeRequestsOutput(t *testing.T, filename string, actual []*adapters.RequestData, expected []httpCall) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Fatalf("%s: MakeRequests had wrong request count. Expected %d, got %d", filename, len(expected), len(actual))
	}
	for i := 0; i < len(actual); i++ {
		diffHttpRequests(t, fmt.Sprintf("%s: httpRequest[%d]", filename, i), actual[i], &(expected[i].Request))
	}
}

func assertErrorList(t *testing.T, description string, actual []error, expected []testSpecExpectedError) {
	t.Helper()

	if len(expected) != len(actual) {
		t.Fatalf("%s had wrong error count. Expected %d, got %d (%v)", description, len(expected), len(actual), actual)
	}
	for i := 0; i < len(actual); i++ {
		if expected[i].Comparison == "literal" {
			if expected[i].Value != actual[i].Error() {
				t.Errorf(`%s error[%d] had wrong message. Expected "%s", got "%s"`, description, i, expected[i].Value, actual[i].Error())
			}
		} else if expected[i].Comparison == "regex" {
			if matched, _ := regexp.MatchString(expected[i].Value, actual[i].Error()); !matched {
				t.Errorf(`%s error[%d] had wrong message. Expected match with regex "%s", got "%s"`, description, i, expected[i].Value, actual[i].Error())
			}
		} else {
			t.Fatalf(`invalid comparison type "%s"`, expected[i].Comparison)
		}
	}
}

func assertMakeBidsOutput(t *testing.T, filename string, actual []*adapters.TypedBid, expected []expectedBid) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("%s: MakeBids returned wrong bid count. Expected %d, got %d", filename, len(expected), len(actual))
	}
	for i := 0; i < len(actual); i++ {
		diffBids(t, fmt.Sprintf("%s:  typedBid[%d]", filename, i), actual[i], &(expected[i]))
	}
}

// diffHttpRequests compares the actual HTTP request data to the expected one.
// It assumes that the request bodies are JSON
func diffHttpRequests(t *testing.T, description string, actual *adapters.RequestData, expected *httpRequest) {
	if actual == nil {
		t.Errorf("Bidders cannot return nil HTTP calls. %s was nil.", description)
		return
	}

	diffStrings(t, fmt.Sprintf("%s.uri", description), actual.Uri, expected.Uri)
	if expected.Headers != nil {
		actualHeader, _ := json.Marshal(actual.Headers)
		expectedHeader, _ := json.Marshal(expected.Headers)
		diffJson(t, description, actualHeader, expectedHeader)
	}
	diffJson(t, description, actual.Body, expected.Body)
}

func diffBids(t *testing.T, description string, actual *adapters.TypedBid, expected *expectedBid) {
	if actual == nil {
		t.Errorf("Bidders cannot return nil TypedBids. %s was nil.", description)
		return
	}

	diffStrings(t, fmt.Sprintf("%s.type", description), string(actual.BidType), string(expected.Type))
	diffOrtbBids(t, fmt.Sprintf("%s.bid", description), actual.Bid, expected.Bid)
}

// diffOrtbBids compares the actual Bid made by the adapter to the expectation from the JSON file.
func diffOrtbBids(t *testing.T, description string, actual *openrtb.Bid, expected json.RawMessage) {
	if actual == nil {
		t.Errorf("Bidders cannot return nil Bids. %s was nil.", description)
		return
	}

	actualJson, err := json.Marshal(actual)
	if err != nil {
		t.Fatalf("%s failed to marshal actual Bid into JSON. %v", description, err)
	}

	diffJson(t, description, actualJson, expected)
}

func diffStrings(t *testing.T, description string, actual string, expected string) {
	if actual != expected {
		t.Errorf(`%s "%s" does not match expected "%s."`, description, actual, expected)
	}
}

// diffJson compares two JSON byte arrays for structural equality. It will produce an error if either
// byte array is not actually JSON.
func diffJson(t *testing.T, description string, actual []byte, expected []byte) {
	if len(actual) == 0 && len(expected) == 0 {
		return
	}
	if len(actual) == 0 || len(expected) == 0 {
		t.Fatalf("%s json diff failed. Expected %d bytes in body, but got %d.", description, len(expected), len(actual))
	}
	diff, err := gojsondiff.New().Compare(actual, expected)
	if err != nil {
		t.Fatalf("%s json diff failed. %v", description, err)
	}

	if diff.Modified() {
		var left interface{}
		if err := json.Unmarshal(actual, &left); err != nil {
			t.Fatalf("%s json did not match, but unmarshalling failed. %v", description, err)
		}
		printer := formatter.NewAsciiFormatter(left, formatter.AsciiFormatterConfig{
			ShowArrayIndex: true,
		})
		output, err := printer.Format(diff)
		if err != nil {
			t.Errorf("%s did not match, but diff formatting failed. %v", description, err)
		} else {
			t.Errorf("%s json did not match expected.\n\n%s", description, output)
		}
	}
}

// assertNoDataRace compares the contents of the reference fields found in the original openrtb.BidRequest to their
// original values to make sure they were not modified and we are not incurring indata races. In order to assert
// no data races occur in the []Imp array, we call assertNoImpsDataRace()
func assertNoDataRace(t *testing.T, bidRequestBefore *openrtb.BidRequest, bidRequestAfter *openrtb.BidRequest, impsBefore []openrtb.Imp, impsAfter []openrtb.Imp) {
	t.Helper()

	// Assert reference fields were not modified by bidder adapter MakeRequests implementation
	assert.Equal(t, bidRequestBefore.Site, bidRequestAfter.Site, "Data race in BidRequest.Site field")
	assert.Equal(t, bidRequestBefore.App, bidRequestAfter.App, "Data race in BidRequest.App field")
	assert.Equal(t, bidRequestBefore.Device, bidRequestAfter.Device, "Data race in BidRequest.Device field")
	assert.Equal(t, bidRequestBefore.User, bidRequestAfter.User, "Data race in BidRequest.User field")
	assert.Equal(t, bidRequestBefore.Source, bidRequestAfter.Source, "Data race in BidRequest.Source field")
	assert.Equal(t, bidRequestBefore.Regs, bidRequestAfter.Regs, "Data race in BidRequest.Regs field")

	// Assert slice fields were not modified by bidder adapter MakeRequests implementation
	assert.ElementsMatch(t, bidRequestBefore.WSeat, bidRequestAfter.WSeat, "Data race in BidRequest.[]WSeat array")
	assert.ElementsMatch(t, bidRequestBefore.BSeat, bidRequestAfter.BSeat, "Data race in BidRequest.[]BSeat array")
	assert.ElementsMatch(t, bidRequestBefore.Cur, bidRequestAfter.Cur, "Data race in BidRequest.[]Cur array")
	assert.ElementsMatch(t, bidRequestBefore.WLang, bidRequestAfter.WLang, "Data race in BidRequest.[]WLang array")
	assert.ElementsMatch(t, bidRequestBefore.BCat, bidRequestAfter.BCat, "Data race in BidRequest.[]BCat array")
	assert.ElementsMatch(t, bidRequestBefore.BAdv, bidRequestAfter.BAdv, "Data race in BidRequest.[]BAdv array")
	assert.ElementsMatch(t, bidRequestBefore.BApp, bidRequestAfter.BApp, "Data race in BidRequest.[]BApp array")
	assert.ElementsMatch(t, bidRequestBefore.Ext, bidRequestAfter.Ext, "Data race in BidRequest.[]Ext array")

	// Assert Imps separately
	assertNoImpsDataRace(t, impsBefore, impsAfter)
}

// assertNoImpsDataRace compares the contents of the reference fields found in the original openrtb.Imp objects to
// their original values to make sure they were not modified and we are not incurring in data races.
func assertNoImpsDataRace(t *testing.T, impsBefore []openrtb.Imp, impsAfter []openrtb.Imp) {
	t.Helper()

	assert.Len(t, impsAfter, len(impsBefore), "Original []Imp array was modified and length is not equal to original after MakeRequests was called")

	// Assert no data races occured in individual Imp elements
	for i := 0; i < len(impsBefore); i++ {
		assert.Equal(t, impsBefore[i].Banner, impsAfter[i].Banner, "Data race in bidRequest.Imp[%d].Banner field", i)
		assert.Equal(t, impsBefore[i].Video, impsAfter[i].Video, "Data race in bidRequest.Imp[%d].Video field", i)
		assert.Equal(t, impsBefore[i].Audio, impsAfter[i].Audio, "Data race in bidRequest.Imp[%d].Audio field", i)
		assert.Equal(t, impsBefore[i].Native, impsAfter[i].Native, "Data race in bidRequest.Imp[%d].Native field", i)
		assert.Equal(t, impsBefore[i].PMP, impsAfter[i].PMP, "Data race in bidRequest.Imp[%d].PMP field", i)
		assert.Equal(t, impsBefore[i].Secure, impsAfter[i].Secure, "Data race in bidRequest.Imp[%d].Secure field", i)

		assert.ElementsMatch(t, impsBefore[i].Metric, impsAfter[i].Metric, "Data race in bidRequest.Imp[%d].[]Metric array", i)
		assert.ElementsMatch(t, impsBefore[i].IframeBuster, impsAfter[i].IframeBuster, "Data race in bidRequest.Imp[%d].[]IframeBuster array", i)
	}
}
