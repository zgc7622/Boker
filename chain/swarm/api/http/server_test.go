// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package http_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Bokerchain/Boker/chain/common"
	"github.com/Bokerchain/Boker/chain/swarm/api"
	swarm "github.com/Bokerchain/Boker/chain/swarm/api/client"
	"github.com/Bokerchain/Boker/chain/swarm/storage"
	"github.com/Bokerchain/Boker/chain/swarm/testutil"
)

func TestBzzrGetPath(t *testing.T) {

	var err error

	testmanifest := []string{
		`{"entries":[{"path":"a/","hash":"674af7073604ebfc0282a4ab21e5ef1a3c22913866879ebc0816f8a89896b2ed","contentType":"application/bzz-manifest+json","status":0}]}`,
		`{"entries":[{"path":"a","hash":"011b4d03dd8c01f1049143cf9c4c817e4b167f1d1b83e5c6f0f10d89ba1e7bce","contentType":"","status":0},{"path":"b/","hash":"0a87b1c3e4bf013686cdf107ec58590f2004610ee58cc2240f26939f691215f5","contentType":"application/bzz-manifest+json","status":0}]}`,
		`{"entries":[{"path":"b","hash":"011b4d03dd8c01f1049143cf9c4c817e4b167f1d1b83e5c6f0f10d89ba1e7bce","contentType":"","status":0},{"path":"c","hash":"011b4d03dd8c01f1049143cf9c4c817e4b167f1d1b83e5c6f0f10d89ba1e7bce","contentType":"","status":0}]}`,
	}

	testrequests := make(map[string]int)
	testrequests["/"] = 0
	testrequests["/a/"] = 1
	testrequests["/a/b/"] = 2
	testrequests["/x"] = 0
	testrequests[""] = 0

	expectedfailrequests := []string{"", "/x"}

	reader := [3]*bytes.Reader{}

	key := [3]storage.Key{}

	srv := testutil.NewTestSwarmServer(t)
	defer srv.Close()

	wg := &sync.WaitGroup{}

	for i, mf := range testmanifest {
		reader[i] = bytes.NewReader([]byte(mf))
		key[i], err = srv.Dpa.Store(reader[i], int64(len(mf)), wg, nil)
		if err != nil {
			t.Fatal(err)
		}
		wg.Wait()
	}

	_, err = http.Get(srv.URL + "/bzzr:/" + common.ToHex(key[0])[2:] + "/a")
	if err != nil {
		t.Fatalf("Failed to connect to proxy: %v", err)
	}

	for k, v := range testrequests {
		var resp *http.Response
		var respbody []byte

		url := srv.URL + "/bzzr:/"
		if k[:] != "" {
			url += common.ToHex(key[0])[2:] + "/" + k[1:] + "?content_type=text/plain"
		}
		resp, err = http.Get(url)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		respbody, err = ioutil.ReadAll(resp.Body)

		if string(respbody) != testmanifest[v] {
			isexpectedfailrequest := false

			for _, r := range expectedfailrequests {
				if k[:] == r {
					isexpectedfailrequest = true
				}
			}
			if !isexpectedfailrequest {
				t.Fatalf("Response body does not match, expected: %v, got %v", testmanifest[v], string(respbody))
			}
		}
	}

	nonhashtests := []string{
		srv.URL + "/bzz:/name",
		srv.URL + "/bzzi:/nonhash",
		srv.URL + "/bzzr:/nonhash",
	}

	nonhashresponses := []string{
		"error resolving name: no DNS to resolve name: &#34;name&#34;",
		"error resolving nonhash: immutable address not a content hash: &#34;nonhash&#34;",
		"error resolving nonhash: no DNS to resolve name: &#34;nonhash&#34;",
	}

	for i, url := range nonhashtests {
		var resp *http.Response
		var respbody []byte

		resp, err = http.Get(url)

		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer resp.Body.Close()
		respbody, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if !strings.Contains(string(respbody), nonhashresponses[i]) {
			t.Fatalf("Non-Hash response body does not match, expected: %v, got: %v", nonhashresponses[i], string(respbody))
		}
	}

}

// TestBzzRootRedirect tests that getting the root path of a manifest without
// a trailing slash gets redirected to include the trailing slash so that
// relative URLs work as expected.
func TestBzzRootRedirect(t *testing.T) {
	srv := testutil.NewTestSwarmServer(t)
	defer srv.Close()

	// create a manifest with some data at the root path
	client := swarm.NewClient(srv.URL)
	data := []byte("data")
	file := &swarm.File{
		ReadCloser: ioutil.NopCloser(bytes.NewReader(data)),
		ManifestEntry: api.ManifestEntry{
			Path:        "",
			ContentType: "text/plain",
			Size:        int64(len(data)),
		},
	}
	hash, err := client.Upload(file, "")
	if err != nil {
		t.Fatal(err)
	}

	// define a CheckRedirect hook which ensures there is only a single
	// redirect to the correct URL
	redirected := false
	httpClient := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if redirected {
				return errors.New("too many redirects")
			}
			redirected = true
			expectedPath := "/bzz:/" + hash + "/"
			if req.URL.Path != expectedPath {
				return fmt.Errorf("expected redirect to %q, got %q", expectedPath, req.URL.Path)
			}
			return nil
		},
	}

	// perform the GET request and assert the response
	res, err := httpClient.Get(srv.URL + "/bzz:/" + hash)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if !redirected {
		t.Fatal("expected GET /bzz:/<hash> to redirect to /bzz:/<hash>/ but it didn't")
	}
	gotData, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotData, data) {
		t.Fatalf("expected response to equal %q, got %q", data, gotData)
	}
}
