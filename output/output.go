package output

import (
	"fmt"
	"io"
	"time"

	"inferlens/metrics"
)

func PrintChatResult(w io.Writer, responseText string, requestMetrics metrics.RequestMetrics) {
	fmt.Fprintln(w, responseText)
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "status: %d\n", requestMetrics.StatusCode)
	fmt.Fprintf(w, "latency: %s\n", requestMetrics.Latency.Round(time.Millisecond))
}
