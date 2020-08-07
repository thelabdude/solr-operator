package solr_api

import "fmt"

func CheckForCollectionsApiError(action string, header SolrResponseHeader) (hasError bool, err error) {
	if header.Status > 0 {
		hasError = true
		err = APIError{
			Detail: fmt.Sprintf("Error occured while calling the Collections api for action=%s", action),
			Status: header.Status,
		}
	}
	return hasError, err
}

func CollectionsAPIError(action string, responseStatus int) error {
	return APIError{
		Detail: fmt.Sprintf("Error occured while calling the Collections api for action=%s", action),
		Status: responseStatus,
	}
}

type APIError struct {
	Detail string
	Status int // API-specific error code
}

func (e APIError) Error() string {
	if e.Status == 0 {
		return e.Detail
	}
	return fmt.Sprintf("Solr response status: %d. %s", e.Status, e.Detail)
}