//go:build go1.21
// +build go1.21

package checksum

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestAddInputMiddleware(t *testing.T) {
	cases := map[string]struct {
		options          InputMiddlewareOptions
		expectErr        string
		expectMiddleware []string
		expectInitialize *SetupInputContext
		expectFinalize   *ComputeInputPayloadChecksum
	}{
		"with trailing checksum": {
			options: InputMiddlewareOptions{
				GetAlgorithm: func(interface{}) (string, bool) {
					return string(AlgorithmCRC32), true
				},
				EnableTrailingChecksum:           true,
				EnableComputeSHA256PayloadHash:   true,
				EnableDecodedContentLengthHeader: true,
			},
			expectMiddleware: []string{
				"test",
				"Initialize stack step",
				"AWSChecksum:SetupInputContext",
				"Serialize stack step",
				"Build stack step",
				"ComputeContentLength",
				"ComputePayloadHash",
				"Finalize stack step",
				"Retry",
				"addInputChecksumTrailer",
				"ResolveEndpointV2",
				"AWSChecksum:ComputeInputPayloadChecksum",
				"Signing",
				"Deserialize stack step",
			},
			expectInitialize: &SetupInputContext{
				GetAlgorithm: func(interface{}) (string, bool) {
					return string(AlgorithmCRC32), true
				},
			},
			expectFinalize: &ComputeInputPayloadChecksum{
				EnableTrailingChecksum:           true,
				EnableComputePayloadHash:         true,
				EnableDecodedContentLengthHeader: true,
			},
		},
		"with checksum required": {
			options: InputMiddlewareOptions{
				GetAlgorithm: func(interface{}) (string, bool) {
					return string(AlgorithmCRC32), true
				},
				EnableTrailingChecksum: true,
				RequireChecksum:        true,
			},
			expectMiddleware: []string{
				"test",
				"Initialize stack step",
				"AWSChecksum:SetupInputContext",
				"Serialize stack step",
				"Build stack step",
				"ComputeContentLength",
				"ComputePayloadHash",
				"Finalize stack step",
				"Retry",
				"addInputChecksumTrailer",
				"ResolveEndpointV2",
				"AWSChecksum:ComputeInputPayloadChecksum",
				"Signing",
				"Deserialize stack step",
			},
			expectInitialize: &SetupInputContext{
				GetAlgorithm: func(interface{}) (string, bool) {
					return string(AlgorithmCRC32), true
				},
			},
			expectFinalize: &ComputeInputPayloadChecksum{
				EnableTrailingChecksum: true,
			},
		},
		"no trailing checksum": {
			options: InputMiddlewareOptions{
				GetAlgorithm: func(interface{}) (string, bool) {
					return string(AlgorithmCRC32), true
				},
			},
			expectMiddleware: []string{
				"test",
				"Initialize stack step",
				"AWSChecksum:SetupInputContext",
				"Serialize stack step",
				"Build stack step",
				"ComputeContentLength",
				"ComputePayloadHash",
				"Finalize stack step",
				"Retry",
				"ResolveEndpointV2",
				"AWSChecksum:ComputeInputPayloadChecksum",
				"Signing",
				"Deserialize stack step",
			},
			expectInitialize: &SetupInputContext{
				GetAlgorithm: func(interface{}) (string, bool) {
					return string(AlgorithmCRC32), true
				},
			},
			expectFinalize: &ComputeInputPayloadChecksum{},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stack := middleware.NewStack("test", smithyhttp.NewStackRequest)

			stack.Build.Add(nopBuildMiddleware("ComputeContentLength"), middleware.After)
			stack.Build.Add(nopBuildMiddleware("ContentChecksum"), middleware.After)
			stack.Build.Add(nopBuildMiddleware("ComputePayloadHash"), middleware.After)
			stack.Finalize.Add(nopFinalizeMiddleware("Retry"), middleware.After)
			stack.Finalize.Add(nopFinalizeMiddleware("ResolveEndpointV2"), middleware.After)
			stack.Finalize.Add(nopFinalizeMiddleware("Signing"), middleware.After)

			err := AddInputMiddleware(stack, c.options)
			if err != nil {
				t.Fatalf("expect no error, got %v", err)
			}

			if diff := cmpDiff(c.expectMiddleware, stack.List()); diff != "" {
				t.Fatalf("expect stack list match:\n%s", diff)
			}

			initializeMiddleware, ok := stack.Initialize.Get((*SetupInputContext)(nil).ID())
			if e, a := (c.expectInitialize != nil), ok; e != a {
				t.Errorf("expect initialize middleware %t, got %t", e, a)
			}
			if c.expectInitialize != nil && ok {
				setupInput := initializeMiddleware.(*SetupInputContext)
				if e, a := c.options.GetAlgorithm != nil, setupInput.GetAlgorithm != nil; e != a {
					t.Fatalf("expect GetAlgorithm %t, got %t", e, a)
				}
				expectAlgo, expectOK := c.options.GetAlgorithm(nil)
				actualAlgo, actualOK := setupInput.GetAlgorithm(nil)
				if e, a := expectAlgo, actualAlgo; e != a {
					t.Errorf("expect %v algorithm, got %v", e, a)
				}
				if e, a := expectOK, actualOK; e != a {
					t.Errorf("expect %v algorithm present, got %v", e, a)
				}
			}

			finalizeMW, ok := stack.Finalize.Get((*ComputeInputPayloadChecksum)(nil).ID())
			if e, a := (c.expectFinalize != nil), ok; e != a {
				t.Errorf("expect build middleware %t, got %t", e, a)
			}
			var ComputeInput *ComputeInputPayloadChecksum
			if c.expectFinalize != nil && ok {
				ComputeInput = finalizeMW.(*ComputeInputPayloadChecksum)
				if e, a := c.expectFinalize.EnableTrailingChecksum, ComputeInput.EnableTrailingChecksum; e != a {
					t.Errorf("expect %v enable trailing checksum, got %v", e, a)
				}
				if e, a := c.expectFinalize.EnableComputePayloadHash, ComputeInput.EnableComputePayloadHash; e != a {
					t.Errorf("expect %v enable compute payload hash, got %v", e, a)
				}
				if e, a := c.expectFinalize.EnableDecodedContentLengthHeader, ComputeInput.EnableDecodedContentLengthHeader; e != a {
					t.Errorf("expect %v enable decoded length header, got %v", e, a)
				}
			}
		})
	}
}

func TestRemoveInputMiddleware(t *testing.T) {
	stack := middleware.NewStack("test", smithyhttp.NewStackRequest)

	stack.Build.Add(nopBuildMiddleware("ComputeContentLength"), middleware.After)
	stack.Build.Add(nopBuildMiddleware("ContentChecksum"), middleware.After)
	stack.Build.Add(nopBuildMiddleware("ComputePayloadHash"), middleware.After)
	stack.Finalize.Add(nopFinalizeMiddleware("Retry"), middleware.After)
	stack.Finalize.Add(nopFinalizeMiddleware("ResolveEndpointV2"), middleware.After)
	stack.Finalize.Add(nopFinalizeMiddleware("Signing"), middleware.After)

	err := AddInputMiddleware(stack, InputMiddlewareOptions{
		EnableTrailingChecksum: true,
	})
	if err != nil {
		t.Fatalf("expect no error, got %v", err)
	}

	RemoveInputMiddleware(stack)

	expectStack := []string{
		"test",
		"Initialize stack step",
		"Serialize stack step",
		"Build stack step",
		"ComputeContentLength",
		"ComputePayloadHash",
		"Finalize stack step",
		"Retry",
		"addInputChecksumTrailer",
		"ResolveEndpointV2",
		"Signing",
		"Deserialize stack step",
	}

	if diff := cmpDiff(expectStack, stack.List()); diff != "" {
		t.Fatalf("expect stack list match:\n%s", diff)
	}
}

func TestAddOutputMiddleware(t *testing.T) {
	cases := map[string]struct {
		options           OutputMiddlewareOptions
		expectErr         string
		expectMiddleware  []string
		expectInitialize  *setupOutputContext
		expectDeserialize *validateOutputPayloadChecksum
	}{
		"validate output": {
			options: OutputMiddlewareOptions{
				GetValidationMode: func(interface{}) (string, bool) {
					return "ENABLED", true
				},
				ValidationAlgorithms: []string{
					"crc32", "sha1", "abc123", "crc32c",
				},
				IgnoreMultipartValidation:     true,
				LogMultipartValidationSkipped: true,
				LogValidationSkipped:          true,
			},
			expectMiddleware: []string{
				"test",
				"Initialize stack step",
				"AWSChecksum:SetupOutputContext",
				"Serialize stack step",
				"Build stack step",
				"Finalize stack step",
				"Deserialize stack step",
				"AWSChecksum:ValidateOutputPayloadChecksum",
			},
			expectInitialize: &setupOutputContext{
				GetValidationMode: func(interface{}) (string, bool) {
					return "ENABLED", true
				},
			},
			expectDeserialize: &validateOutputPayloadChecksum{
				Algorithms: []Algorithm{
					AlgorithmCRC32, AlgorithmSHA1, AlgorithmCRC32C,
				},
				IgnoreMultipartValidation:     true,
				LogMultipartValidationSkipped: true,
				LogValidationSkipped:          true,
			},
		},
		"validate options off": {
			options: OutputMiddlewareOptions{
				GetValidationMode: func(interface{}) (string, bool) {
					return "ENABLED", true
				},
				ValidationAlgorithms: []string{
					"crc32", "sha1", "abc123", "crc32c",
				},
			},
			expectMiddleware: []string{
				"test",
				"Initialize stack step",
				"AWSChecksum:SetupOutputContext",
				"Serialize stack step",
				"Build stack step",
				"Finalize stack step",
				"Deserialize stack step",
				"AWSChecksum:ValidateOutputPayloadChecksum",
			},
			expectInitialize: &setupOutputContext{
				GetValidationMode: func(interface{}) (string, bool) {
					return "ENABLED", true
				},
			},
			expectDeserialize: &validateOutputPayloadChecksum{
				Algorithms: []Algorithm{
					AlgorithmCRC32, AlgorithmSHA1, AlgorithmCRC32C,
				},
			},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			stack := middleware.NewStack("test", smithyhttp.NewStackRequest)

			err := AddOutputMiddleware(stack, c.options)
			if err != nil {
				t.Fatalf("expect no error, got %v", err)
			}

			if diff := cmpDiff(c.expectMiddleware, stack.List()); diff != "" {
				t.Fatalf("expect stack list match:\n%s", diff)
			}

			initializeMiddleware, ok := stack.Initialize.Get((*setupOutputContext)(nil).ID())
			if e, a := (c.expectInitialize != nil), ok; e != a {
				t.Errorf("expect initialize middleware %t, got %t", e, a)
			}
			if c.expectInitialize != nil && ok {
				setupOutput := initializeMiddleware.(*setupOutputContext)
				if e, a := c.options.GetValidationMode != nil, setupOutput.GetValidationMode != nil; e != a {
					t.Fatalf("expect GetValidationMode %t, got %t", e, a)
				}
				expectMode, expectOK := c.options.GetValidationMode(nil)
				actualMode, actualOK := setupOutput.GetValidationMode(nil)
				if e, a := expectMode, actualMode; e != a {
					t.Errorf("expect %v mode, got %v", e, a)
				}
				if e, a := expectOK, actualOK; e != a {
					t.Errorf("expect %v mode present, got %v", e, a)
				}
			}

			deserializeMiddleware, ok := stack.Deserialize.Get((*validateOutputPayloadChecksum)(nil).ID())
			if e, a := (c.expectDeserialize != nil), ok; e != a {
				t.Errorf("expect deserialize middleware %t, got %t", e, a)
			}
			if c.expectDeserialize != nil && ok {
				validateOutput := deserializeMiddleware.(*validateOutputPayloadChecksum)
				if diff := cmpDiff(c.expectDeserialize.Algorithms, validateOutput.Algorithms); diff != "" {
					t.Errorf("expect algorithms match:\n%s", diff)
				}
				if e, a := c.expectDeserialize.IgnoreMultipartValidation, validateOutput.IgnoreMultipartValidation; e != a {
					t.Errorf("expect %v ignore multipart checksum, got %v", e, a)
				}
				if e, a := c.expectDeserialize.LogMultipartValidationSkipped, validateOutput.LogMultipartValidationSkipped; e != a {
					t.Errorf("expect %v log multipart skipped, got %v", e, a)
				}
				if e, a := c.expectDeserialize.LogValidationSkipped, validateOutput.LogValidationSkipped; e != a {
					t.Errorf("expect %v log validation skipped, got %v", e, a)
				}
			}
		})
	}
}

func TestRemoveOutputMiddleware(t *testing.T) {
	stack := middleware.NewStack("test", smithyhttp.NewStackRequest)

	err := AddOutputMiddleware(stack, OutputMiddlewareOptions{})
	if err != nil {
		t.Fatalf("expect no error, got %v", err)
	}

	RemoveOutputMiddleware(stack)

	expectStack := []string{
		"test",
		"Initialize stack step",
		"Serialize stack step",
		"Build stack step",
		"Finalize stack step",
		"Deserialize stack step",
	}

	if diff := cmpDiff(expectStack, stack.List()); diff != "" {
		t.Fatalf("expect stack list match:\n%s", diff)
	}
}

func setSerializedRequest(req *smithyhttp.Request) middleware.SerializeMiddleware {
	return middleware.SerializeMiddlewareFunc("OperationSerializer",
		func(ctx context.Context, input middleware.SerializeInput, next middleware.SerializeHandler) (
			middleware.SerializeOutput, middleware.Metadata, error,
		) {
			input.Request = req
			return next.HandleSerialize(ctx, input)
		})
}

func nopBuildMiddleware(id string) middleware.BuildMiddleware {
	return middleware.BuildMiddlewareFunc(id,
		func(ctx context.Context, input middleware.BuildInput, next middleware.BuildHandler) (
			middleware.BuildOutput, middleware.Metadata, error,
		) {
			return next.HandleBuild(ctx, input)
		})
}

func nopFinalizeMiddleware(id string) middleware.FinalizeMiddleware {
	return middleware.FinalizeMiddlewareFunc(id,
		func(ctx context.Context, input middleware.FinalizeInput, next middleware.FinalizeHandler) (
			middleware.FinalizeOutput, middleware.Metadata, error,
		) {
			return next.HandleFinalize(ctx, input)
		})
}

func cmpDiff(e, a interface{}) string {
	if !reflect.DeepEqual(e, a) {
		return fmt.Sprintf("%v != %v", e, a)
	}
	return ""
}
