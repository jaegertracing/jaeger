package es

// Some of the functions of elastic.BulkIndexRequest violate golint rules.

// Id calls this function to internal service.
func (i IndexServiceWrapper) Id(id string) IndexService {
	return WrapESIndexService(i.bulkIndexReq.Id(id), i.bulkService)
}

// BodyJson calls this function to internal service.
func (i IndexServiceWrapper) BodyJson(body interface{}) IndexService {
	return WrapESIndexService(i.bulkIndexReq.Doc(body), i.bulkService)
}
