package newrelic

type GraphQLResponse struct {
	Data struct {
		Actor struct {
			ErrorsInbox struct {
				ErrorGroups struct {
					TotalCount int           `json:"totalCount"`
					Results    []ErrorGroup  `json:"results"`
					NextCursor *string       `json:"nextCursor"`
				} `json:"errorGroups"`
			} `json:"errorsInbox"`
			Account struct {
				NRQL struct {
					Results []map[string]interface{} `json:"results"`
				} `json:"nrql"`
			} `json:"account"`
		} `json:"actor"`
	} `json:"data"`
}

type ErrorGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Message     string `json:"message"`
	State       string `json:"state"`
	Occurrences struct {
		TotalCount    int `json:"totalCount"`
		ExpectedCount int `json:"expectedCount"`
	} `json:"occurrences"`
	FirstSeenAt int64  `json:"firstSeenAt"`
	LastSeenAt  int64  `json:"lastSeenAt"`
	EventsQuery string `json:"eventsQuery"`
}

type OccurrenceDetail struct {
	ErrorClass      string  `json:"error.class"`
	ErrorExpected   bool    `json:"error.expected"`
	ErrorMessage    string  `json:"error.message"`
	Host            string  `json:"host"`
	RequestURI      string  `json:"request.uri"`
	Timestamp       float64 `json:"timestamp"`
	TransactionName string  `json:"transactionUiName"`
}
