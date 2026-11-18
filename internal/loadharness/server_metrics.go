package loadharness

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const maxServerMetricsBytes = 4 << 20

var selectedServerMetrics = map[string]struct{}{
	"rugram_mtproto_raw_connections":                          {},
	"rugram_mtproto_connections_active":                       {},
	"rugram_mtproto_sessions":                                 {},
	"rugram_mtproto_logical_sessions":                         {},
	"rugram_mtproto_logical_outbox_frames":                    {},
	"rugram_mtproto_logical_outbox_bytes":                     {},
	"rugram_mtproto_logical_outbox_acked_frames_total":        {},
	"rugram_mtproto_logical_outbox_acked_bytes_total":         {},
	"rugram_mtproto_logical_outbox_retained_seconds_count":    {},
	"rugram_mtproto_logical_outbox_retained_seconds_sum":      {},
	"rugram_mtproto_pending_push_bytes":                       {},
	"rugram_mtproto_inbound_rpc_tasks":                        {},
	"rugram_mtproto_inbound_rpc_bytes":                        {},
	"rugram_mtproto_rpc_delivery_hook_workers":                {},
	"rugram_mtproto_rpc_delivery_hook_capacity":               {},
	"rugram_mtproto_rpc_delivery_hook_reserved":               {},
	"rugram_mtproto_rpc_delivery_hook_queued":                 {},
	"rugram_mtproto_rpc_delivery_hook_running":                {},
	"rugram_mtproto_rpc_delivery_hook_completed_total":        {},
	"rugram_mtproto_rpc_delivery_hook_rejected_total":         {},
	"rugram_mtproto_rpc_delivery_hook_panics_total":           {},
	"rugram_mtproto_rpc_delivery_hook_duration_seconds_total": {},
	"rugram_mtproto_inbound_frame_bytes":                      {},
	"rugram_mtproto_outbound_tracked_bytes":                   {},
	"rugram_mtproto_outbound_write_bytes":                     {},
	"rugram_mtproto_rpc_execution_owners":                     {},
	"rugram_mtproto_rpc_execution_reserved_entries":           {},
	"rugram_mtproto_rpc_execution_receipts":                   {},
	"rugram_mtproto_rpc_execution_receipt_budget_bytes":       {},
	"rugram_mtproto_rpc_execution_subscribers":                {},
	"rugram_mtproto_rpc_result_inner_bytes_total":             {},
	"rugram_mtproto_rpc_result_wire_bytes_total":              {},
	"rugram_mtproto_rpc_result_delivered_total":               {},
	"rugram_mtproto_rpc_result_delivered_bytes_total":         {},
	"rugram_go_goroutines":                                    {},
	"rugram_process_cpu_seconds":                              {},
	"rugram_go_scheduler_busy_seconds":                        {},
	"rugram_go_gc_cycles":                                     {},
	"rugram_go_gc_pause_seconds":                              {},
	"rugram_go_heap_alloc_bytes":                              {},
	"rugram_go_heap_inuse_bytes":                              {},
	"rugram_go_heap_objects":                                  {},
	"rugram_go_stack_inuse_bytes":                             {},
	"rugram_go_sys_bytes":                                     {},
	"rugram_postgres_pool_connections":                        {},
	"rugram_postgres_pool_acquire_count":                      {},
	"rugram_postgres_pool_acquire_wait_seconds":               {},
	"rugram_postgres_pool_empty_acquire_count":                {},
	"rugram_postgres_pool_canceled_acquire_count":             {},
	"rugram_postgres_pool_max_connections":                    {},
	"rugram_redis_pool_connections":                           {},
	"rugram_redis_pool_hits":                                  {},
	"rugram_redis_pool_misses":                                {},
	"rugram_redis_pool_pending_requests":                      {},
	"rugram_redis_pool_timeouts":                              {},
	"rugram_redis_pool_wait_count":                            {},
	"rugram_redis_pool_wait_seconds":                          {},
	"rugram_rpc_db_queries_total":                             {},
	"rugram_rpc_db_errors_total":                              {},
	"rugram_rpc_db_time_seconds_sum":                          {},
	"rugram_rpc_db_time_seconds_count":                        {},
	"rugram_channel_difference_cache_entries":                 {},
	"rugram_channel_difference_cache_weight_bytes":            {},
	"rugram_channel_difference_cache_hits":                    {},
	"rugram_channel_difference_cache_misses":                  {},
	"rugram_channel_difference_cache_loads":                   {},
	"rugram_channel_difference_cache_load_errors":             {},
	"rugram_bootstrap_ready_batches_total":                    {},
	"rugram_bootstrap_ready_selectors_total":                  {},
	"rugram_bootstrap_ready_pending":                          {},
	"rugram_active_channel_ids_cache_total":                   {},
	"rugram_active_channel_ids_batches_total":                 {},
	"rugram_active_channel_ids_selectors_total":               {},
	"rugram_active_channel_ids_rows_total":                    {},
	"rugram_active_channel_ids_pending":                       {},
	"rugram_presence_last_seen_batches_total":                 {},
	"rugram_presence_last_seen_updates_total":                 {},
	"rugram_presence_last_seen_submitted_total":               {},
	"rugram_presence_last_seen_pending":                       {},
	"rugram_presence_last_seen_overflow_total":                {},
	"rugram_presence_last_seen_drain_dropped_total":           {},
	"rugram_metrics_dropped_observations_total":               {},
}

type serverMetricsClient struct {
	url     string
	client  *http.Client
	success atomic.Uint64
	errors  atomic.Uint64
}

func newServerMetricsClient(url string) *serverMetricsClient {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	return &serverMetricsClient{url: url, client: &http.Client{Timeout: 5 * time.Second}}
}

func (c *serverMetricsClient) scrape(ctx context.Context) (map[string]float64, error) {
	if c == nil {
		return nil, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		c.errors.Add(1)
		return nil, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		c.errors.Add(1)
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		c.errors.Add(1)
		return nil, fmt.Errorf("metrics HTTP status %d", response.StatusCode)
	}
	reader := bufio.NewScanner(io.LimitReader(response.Body, maxServerMetricsBytes))
	reader.Buffer(make([]byte, 64<<10), 1<<20)
	values := make(map[string]float64, len(selectedServerMetrics))
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		if idx := strings.IndexByte(name, '{'); idx >= 0 {
			name = name[:idx]
		}
		if _, ok := selectedServerMetrics[name]; !ok {
			continue
		}
		value, err := strconv.ParseFloat(fields[1], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		// Reports need bounded, comparable capacity signals, not an unbounded copy
		// of Prometheus label series. Aggregate every selected family into one
		// key. Response-byte and DB-work families additionally retain only their
		// code-owned method label; bounded pool/session families retain their state
		// label. This supports attribution without copying auth/session/user
		// cardinality.
		values[name] += value
		if isPerMethodOutcomeServerMetric(name) {
			method, methodOK := prometheusLabelValue(fields[0], "method")
			outcome, outcomeOK := prometheusLabelValue(fields[0], "outcome")
			if methodOK && outcomeOK {
				values[name+`{method="`+method+`",outcome="`+outcome+`"}`] += value
			}
		} else if isPerMethodServerMetric(name) {
			if method, ok := prometheusLabelValue(fields[0], "method"); ok {
				values[name+`{method="`+method+`"}`] += value
			}
		} else if isOutcomeServerMetric(name) {
			if outcome, ok := prometheusLabelValue(fields[0], "outcome"); ok {
				values[name+`{outcome="`+outcome+`"}`] += value
			}
		}
		if isStateServerMetric(name) {
			if state, ok := prometheusLabelValue(fields[0], "state"); ok {
				values[name+`{state="`+state+`"}`] += value
			}
		}
	}
	if err := reader.Err(); err != nil {
		c.errors.Add(1)
		return nil, err
	}
	c.success.Add(1)
	return values, nil
}

// waitForPresenceLastSeenSettlement waits until every expected lifecycle event
// has reached the server-owned batch queue and all accepted work has drained.
// It is report-only synchronization: it does not participate in RPC success or
// alter the server's presence semantics.
func (c *serverMetricsClient) waitForPresenceLastSeenSettlement(
	ctx context.Context,
	baselineSubmitted float64,
	expectedSubmitted uint64,
	timeout time.Duration,
) (map[string]float64, error) {
	if c == nil {
		return nil, nil
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(100 * time.Millisecond)
	defer poll.Stop()
	var last map[string]float64
	for {
		sample, err := c.scrape(ctx)
		if err != nil {
			return last, err
		}
		last = sample
		submitted := metricValue(sample, "rugram_presence_last_seen_submitted_total") - baselineSubmitted
		pending := metricValue(sample, "rugram_presence_last_seen_pending")
		bootstrapPending := metricValue(sample, "rugram_bootstrap_ready_pending")
		activeChannelIDsPending := metricValue(sample, "rugram_active_channel_ids_pending")
		if submitted >= float64(expectedSubmitted) && pending == 0 && bootstrapPending == 0 && activeChannelIDsPending == 0 {
			return sample, nil
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-deadline.C:
			return last, fmt.Errorf("startup settlement timeout: presence submitted=%.0f expected=%d pending=%.0f bootstrap_pending=%.0f active_channel_ids_pending=%.0f", submitted, expectedSubmitted, pending, bootstrapPending, activeChannelIDsPending)
		case <-poll.C:
		}
	}
}

func isOutcomeServerMetric(name string) bool {
	switch name {
	case "rugram_presence_last_seen_batches_total", "rugram_presence_last_seen_updates_total",
		"rugram_bootstrap_ready_batches_total", "rugram_bootstrap_ready_selectors_total",
		"rugram_active_channel_ids_cache_total", "rugram_active_channel_ids_batches_total",
		"rugram_active_channel_ids_selectors_total":
		return true
	default:
		return false
	}
}

func isPerMethodServerMetric(name string) bool {
	switch name {
	case "rugram_mtproto_rpc_result_inner_bytes_total",
		"rugram_mtproto_rpc_result_wire_bytes_total",
		"rugram_rpc_db_queries_total",
		"rugram_rpc_db_errors_total",
		"rugram_rpc_db_time_seconds_sum",
		"rugram_rpc_db_time_seconds_count":
		return true
	default:
		return false
	}
}

func isPerMethodOutcomeServerMetric(name string) bool {
	switch name {
	case "rugram_mtproto_rpc_result_delivered_total", "rugram_mtproto_rpc_result_delivered_bytes_total":
		return true
	default:
		return false
	}
}

func isStateServerMetric(name string) bool {
	switch name {
	case "rugram_mtproto_sessions", "rugram_mtproto_logical_sessions",
		"rugram_postgres_pool_connections", "rugram_redis_pool_connections":
		return true
	default:
		return false
	}
}

func prometheusLabelValue(series, label string) (string, bool) {
	needle := label + `="`
	start := strings.Index(series, needle)
	if start < 0 {
		return "", false
	}
	start += len(needle)
	end := start
	for end < len(series) {
		if series[end] == '"' && (end == start || series[end-1] != '\\') {
			return series[start:end], true
		}
		end++
	}
	return "", false
}

func (c *serverMetricsClient) successes() uint64 {
	if c == nil {
		return 0
	}
	return c.success.Load()
}

func (c *serverMetricsClient) failures() uint64 {
	if c == nil {
		return 0
	}
	return c.errors.Load()
}
