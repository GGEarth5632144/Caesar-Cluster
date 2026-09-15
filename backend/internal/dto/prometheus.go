package dto

type PrometheusResponse struct {
    Status string `json:"status"`
    Data   struct {
        ResultType string `json:"resultType"`
        Result     []struct {
            Metric map[string]string `json:"metric"`
            Value  []interface{}     `json:"value"`  
            Values [][]interface{}   `json:"values"` 
        } `json:"result"`
    } `json:"data"`
}