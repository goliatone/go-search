package errs

import (
	stderrors "errors"
	"net/http"

	goerrors "github.com/goliatone/go-errors"
	"github.com/goliatone/go-search/pkg/types"
)

func UnknownIndex(index string, metadata map[string]any) error {
	return goerrors.New("unknown search index", goerrors.CategoryNotFound).
		WithCode(http.StatusNotFound).
		WithTextCode(types.TextCodeUnknownIndex).
		WithMetadata(merge(metadata, map[string]any{"index": index}))
}

func UnsupportedCapability(capability string, metadata map[string]any) error {
	return goerrors.New("unsupported search capability", goerrors.CategoryBadInput).
		WithCode(http.StatusBadRequest).
		WithTextCode(types.TextCodeUnsupportedCapability).
		WithMetadata(merge(metadata, map[string]any{"capability": capability}))
}

func InvalidFilter(message string, metadata map[string]any) error {
	return goerrors.New(message, goerrors.CategoryValidation).
		WithCode(http.StatusBadRequest).
		WithTextCode(types.TextCodeInvalidFilter).
		WithMetadata(metadata)
}

func InvalidSort(message string, metadata map[string]any) error {
	return goerrors.New(message, goerrors.CategoryValidation).
		WithCode(http.StatusBadRequest).
		WithTextCode(types.TextCodeInvalidSort).
		WithMetadata(metadata)
}

func IndexingSourceMissing(index string, metadata map[string]any) error {
	return goerrors.New("indexing source is not registered", goerrors.CategoryNotFound).
		WithCode(http.StatusNotFound).
		WithTextCode(types.TextCodeIndexingSourceMissing).
		WithMetadata(merge(metadata, map[string]any{"index": index}))
}

func ProjectorFailure(err error, metadata map[string]any) error {
	return goerrors.Wrap(err, goerrors.CategoryOperation, "projector failure").
		WithCode(http.StatusBadGateway).
		WithTextCode(types.TextCodeProjectorFailure).
		WithMetadata(metadata)
}

func SchemaMismatch(message string, metadata map[string]any) error {
	return goerrors.New(message, goerrors.CategoryConflict).
		WithCode(http.StatusConflict).
		WithTextCode(types.TextCodeSchemaMismatch).
		WithMetadata(metadata)
}

func FeatureDisabled(message string, metadata map[string]any) error {
	return goerrors.New(message, goerrors.CategoryAuthz).
		WithCode(http.StatusForbidden).
		WithTextCode(types.TextCodeFeatureDisabled).
		WithMetadata(metadata)
}

func RankingFailure(err error, metadata map[string]any) error {
	return goerrors.Wrap(err, goerrors.CategoryInternal, "ranking failure").
		WithCode(http.StatusInternalServerError).
		WithTextCode(types.TextCodeRankingFailure).
		WithMetadata(metadata)
}

func ConfigurationError(message string, metadata map[string]any) error {
	return goerrors.New(message, goerrors.CategoryBadInput).
		WithCode(http.StatusBadRequest).
		WithTextCode("SEARCH_CONFIGURATION_ERROR").
		WithMetadata(metadata)
}

func Wrap(err error, metadata map[string]any) error {
	if err == nil {
		return nil
	}
	var rich *goerrors.Error
	if stderrors.As(err, &rich) {
		return rich.WithMetadata(metadata)
	}
	return goerrors.Wrap(err, goerrors.CategoryInternal, err.Error()).WithMetadata(metadata)
}

func merge(base map[string]any, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
