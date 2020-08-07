package solr_api

import (
	"encoding/json"
	"fmt"
	solr "github.com/bloomberg/solr-operator/api/v1beta1"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"net/url"
)

type SolrAsyncResponse struct {
	ResponseHeader SolrResponseHeader `json:"responseHeader"`

	// +optional
	RequestId string `json:"requestId"`

	// +optional
	Status SolrAsyncStatus `json:"status"`
}

type SolrResponseHeader struct {
	Status int `json:"status"`

	QTime int `json:"QTime"`
}

type SolrAsyncStatus struct {
	// Possible states can be found here: https://github.com/apache/lucene-solr/blob/1d85cd783863f75cea133fb9c452302214165a4d/solr/solrj/src/java/org/apache/solr/client/solrj/response/RequestStatusState.java
	AsyncState string `json:"state"`

	Message string `json:"msg"`
}

func CallCollectionsApi(cloud string, namespace string, urlParams url.Values, response interface{}) (err error) {
	cloudUrl := solr.InternalURLForCloud(cloud, namespace)

	urlParams.Set("wt", "json")

	cloudUrl = cloudUrl + "/solr/admin/collections?" + urlParams.Encode()

	resp := &http.Response{}
	if resp, err = http.Get(cloudUrl); err != nil {
		return err
	}

	defer resp.Body.Close()

	if err == nil && resp.StatusCode != 200 {
		b, _ := ioutil.ReadAll(resp.Body)
		err = errors.NewServiceUnavailable(fmt.Sprintf("Recieved bad response code of %d from solr with response: %s", resp.StatusCode, string(b)))
	}

	if err == nil {
		json.NewDecoder(resp.Body).Decode(&response)
	}

	return err
}