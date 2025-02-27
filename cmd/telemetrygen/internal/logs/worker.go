// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logs

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/atomic"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type worker struct {
	running        *atomic.Bool    // pointer to shared flag that indicates it's time to stop the test
	numLogs        int             // how many logs the worker has to generate (only when duration==0)
	totalDuration  time.Duration   // how long to run the test for (overrides `numLogs`)
	limitPerSecond rate.Limit      // how many logs per second to generate
	wg             *sync.WaitGroup // notify when done
	logger         *zap.Logger     // logger
	index          int             // worker index
}

func (w worker) simulateLogs(res *resource.Resource, exporter exporter) {
	limiter := rate.NewLimiter(w.limitPerSecond, 1)
	var i int64

	for w.running.Load() {
		logs := plog.NewLogs()
		nRes := logs.ResourceLogs().AppendEmpty().Resource()
		attrs := res.Attributes()
		for _, attr := range attrs {
			nRes.Attributes().PutStr(string(attr.Key), attr.Value.AsString())
		}
		log := logs.ResourceLogs().At(0).ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
		log.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		log.SetDroppedAttributesCount(1)
		log.SetSeverityNumber(plog.SeverityNumberInfo)
		log.SetSeverityText("Info")
		lattrs := log.Attributes()
		lattrs.PutStr("app", "server")

		if err := exporter.export(logs); err != nil {
			w.logger.Fatal("exporter failed", zap.Error(err))
		}
		if err := limiter.Wait(context.Background()); err != nil {
			w.logger.Fatal("limiter wait failed, retry", zap.Error(err))
		}

		i++
		if w.numLogs != 0 && i >= int64(w.numLogs) {
			break
		}
	}

	w.logger.Info("logs generated", zap.Int64("logs", i))
	w.wg.Done()
}
