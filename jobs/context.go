package jobs

import (
	"context"
	"encoding/json"

	queuecmd "github.com/goliatone/go-job/queue/command"
)

type dispatchContextKey string

const (
	dispatchOperationKeyContextKey dispatchContextKey = "search.jobs.operation_key"
)

func withOperationKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, dispatchOperationKeyContextKey, key)
}

func operationKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(dispatchOperationKeyContextKey).(string)
	return value
}

func dispatchMetadataFromParams(params map[string]any) operationDispatchMetadata {
	if len(params) == 0 {
		return operationDispatchMetadata{}
	}
	raw, ok := params[queuecmd.DispatchMetadataKey]
	if !ok {
		return operationDispatchMetadata{}
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return operationDispatchMetadata{}
	}
	var out operationDispatchMetadata
	if err := json.Unmarshal(body, &out); err != nil {
		return operationDispatchMetadata{}
	}
	return out
}

func clonePayload(params map[string]any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]any, len(params))
	for key, value := range params {
		if key == queuecmd.DispatchMetadataKey {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
