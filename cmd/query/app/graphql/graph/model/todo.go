package model

type Todo struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Done   bool   `json:"done"`
	UserID string `json:"user"`
}


type NewTodo struct {
	Text   string `json:"text"`
	UserID string `json:"userId"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

//type Span struct {
//	ID            string `json:"id"`
//	TraceID       string `json:"traceId"`
//	OperationName string `json:"operationName"`
//}
