package dto

type PrometheusResponse struct {
    Status string `json:"status"`
    Data   struct {
        ResultType string `json:"resultType"`
        Result     []struct {
            Metric map[string]string `json:"metric"`
            Value  []interface{}     `json:"value"`  // สำหรับ Snapshot
            Values [][]interface{}   `json:"values"` // [เพิ่มบรรทัดนี้] สำหรับข้อมูลกราฟย้อนหลัง
        } `json:"result"`
    } `json:"data"`
}