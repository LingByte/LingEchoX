// Command onnx-intent-demo exercises pkg/intentonnx (ONNX Runtime + tokenizer + routing).
//
// Examples:
//
//	export ONNXRUNTIME_SHARED_LIBRARY_PATH=/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib
//	go run ./cmd/onnx-intent-demo -model ./model_q4.onnx -info
//	go run ./cmd/onnx-intent-demo -model ./model_q4.onnx -tokenizer ./tokenizer.json -text "我要转人工" -seq 128
//	go run ./cmd/onnx-intent-demo ... -text "..." -json
//
// Integration: use [github.com/LingByte/SoulNexus/pkg/intentonnx] from your service; never mix
// RouteOutput.Reply with an LLM answer for the same user turn (see package doc).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LingByte/SoulNexus/pkg/intentonnx"
	ort "github.com/yalue/onnxruntime_go"
)

func main() {
	ortLib := flag.String("ort", strings.TrimSpace(os.Getenv("ONNXRUNTIME_SHARED_LIBRARY_PATH")), "path to onnxruntime shared library (dylib/so); or set ONNXRUNTIME_SHARED_LIBRARY_PATH")
	modelPath := flag.String("model", "", "path to .onnx file")
	infoOnly := flag.Bool("info", false, "print model inputs/outputs and exit")
	seqLen := flag.Int("seq", 128, "sequence length for [batch,seq] inputs")
	batch := flag.Int("batch", 1, "batch size (use 1 with -text)")
	tokenizerPath := flag.String("tokenizer", "", "path to tokenizer.json (required with -text)")
	text := flag.String("text", "", "user utterance; uses pkg/intentonnx when set")
	labelsCSV := flag.String("labels", "", "optional comma-separated class names (same order as logits)")
	intentsPath := flag.String("intents", "", "optional intents JSON path (empty = package default)")
	noKeywordBias := flag.Bool("no-keywords", false, "disable keyword logit bias")
	uncertainLLM := flag.Bool("uncertain-llm", false, "when set, low-confidence cases defer to LLM (empty reply, AnswerChannelLLM) instead of default_reply")
	jsonOut := flag.Bool("json", false, "print RouteOutput JSON (with -text)")
	coreml := flag.Bool("coreml", false, "append CoreML EP when creating engine")
	flag.Parse()

	if strings.TrimSpace(*modelPath) == "" {
		fmt.Fprintln(os.Stderr, "missing -model")
		flag.Usage()
		os.Exit(2)
	}

	lib := pickORTLib(*ortLib)
	if lib == "" {
		fmt.Fprintln(os.Stderr, "set -ort or ONNXRUNTIME_SHARED_LIBRARY_PATH")
		os.Exit(2)
	}

	if err := intentonnx.InitRuntime(lib); err != nil {
		fmt.Fprintf(os.Stderr, "InitRuntime: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = intentonnx.CloseRuntime() }()

	fmt.Println("onnxruntime:", ort.GetVersion())

	var sessOpts *ort.SessionOptions
	if *coreml {
		o, err := ort.NewSessionOptions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "NewSessionOptions: %v\n", err)
			os.Exit(1)
		}
		if err := o.AppendExecutionProviderCoreMLV2(nil); err != nil {
			_ = o.Destroy()
			fmt.Fprintf(os.Stderr, "AppendExecutionProviderCoreMLV2: %v\n", err)
			os.Exit(1)
		}
		sessOpts = o
		defer func() { _ = o.Destroy() }()
	}

	inputs, outputs, err := ort.GetInputOutputInfoWithOptions(*modelPath, sessOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetInputOutputInfo: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("--- inputs ---")
	for _, in := range inputs {
		fmt.Println(in.String())
	}
	fmt.Println("--- outputs ---")
	for _, out := range outputs {
		fmt.Println(out.String())
	}
	if *infoOnly {
		fmt.Println("ok (-info)")
		return
	}

	textTrim := strings.TrimSpace(*text)
	if textTrim != "" {
		if strings.TrimSpace(*tokenizerPath) == "" {
			fmt.Fprintln(os.Stderr, "-text requires -tokenizer")
			os.Exit(2)
		}
		if *batch != 1 {
			fmt.Fprintln(os.Stderr, "-text requires -batch 1")
			os.Exit(2)
		}
		runText(lib, *modelPath, *tokenizerPath, *seqLen, *coreml, textTrim, *intentsPath, *labelsCSV, *noKeywordBias, *uncertainLLM, *jsonOut)
		return
	}

	runSmoke(*modelPath, inputs, outputs, sessOpts, *seqLen, *batch)
}

func pickORTLib(ortLib string) string {
	lib := strings.TrimSpace(ortLib)
	if lib != "" {
		return lib
	}
	for _, c := range []string{
		"/opt/homebrew/lib/libonnxruntime.dylib",
		"/usr/local/lib/libonnxruntime.dylib",
	} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

func runText(lib, model, tokenizer string, seq int, coreml bool, text, intentsPath, labels string, noKW, uncertainLLM, jsonOut bool) {
	eng, err := intentonnx.NewEngine(intentonnx.Options{
		SharedLibraryPath: lib,
		ModelPath:         model,
		TokenizerPath:     tokenizer,
		SeqLen:            seq,
		UseCoreML:         coreml,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewEngine: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = eng.Close() }()

	cfg, err := intentonnx.LoadIntentConfig(strings.TrimSpace(intentsPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadIntentConfig: %v\n", err)
		os.Exit(1)
	}
	if err := intentonnx.ValidateIntentConfig(cfg, eng.NumClasses()); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	labelsSl := parseLabels(labels)
	t0 := time.Now()
	out, err := eng.Route(text, cfg, intentonnx.RouteOptions{
		DisableKeywordBias: noKW,
		LabelOverrides:     labelsSl,
		UncertainMeansLLM:  uncertainLLM,
		VoiceASRHints:      true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Route: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Route ok in %v\n", time.Since(t0))

	if jsonOut {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "json: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		fmt.Println("ok (see channel: intent = show Reply only; llm = call LLM once, Reply empty)")
		return
	}

	p := out.Prediction
	fmt.Printf("logits: %v\n", p.Logits)
	if len(p.AdjustedLogits) > 0 {
		fmt.Printf("adjusted logits: %v\n", p.AdjustedLogits)
	}
	fmt.Printf("softmax: %v\n", p.Softmax)
	fmt.Println("--- route ---")
	switch out.Channel {
	case intentonnx.AnswerChannelIntent:
		fmt.Println("channel=intent (use Reply only; do not call LLM for this turn)")
	case intentonnx.AnswerChannelLLM:
		fmt.Println("channel=llm (Reply empty — obtain answer from LLM once; do not merge intent text)")
	}
	fmt.Printf("intent_index=%d name=%q confidence=%.4f config_fallback=%v\n",
		p.IntentIndex, p.IntentName, p.Confidence, p.UsedConfigFallback)
	fmt.Println("--- reply ---")
	if out.Reply != "" {
		fmt.Println(out.Reply)
	} else {
		fmt.Println("(none — defer to LLM)")
	}
	fmt.Println("ok")
}

func runSmoke(model string, inputs, outputs []ort.InputOutputInfo, sessOpts *ort.SessionOptions, seq, batch int) {
	inNames := make([]string, len(inputs))
	for i := range inputs {
		inNames[i] = inputs[i].Name
	}
	outNames := make([]string, len(outputs))
	for i := range outputs {
		outNames[i] = outputs[i].Name
	}

	sess, err := ort.NewDynamicAdvancedSession(model, inNames, outNames, sessOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewDynamicAdvancedSession: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = sess.Destroy() }()

	inVals := make([]ort.Value, 0, len(inputs))
	defer destroyVals(inVals)

	for _, meta := range inputs {
		if meta.OrtValueType != ort.ONNXTypeTensor {
			fmt.Fprintf(os.Stderr, "unsupported input %q\n", meta.Name)
			os.Exit(1)
		}
		shape, err := concreteShapeSmoke(meta.Dimensions, seq, batch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "shape: %v\n", err)
			os.Exit(1)
		}
		n := int(shape.FlattenedSize())
		switch meta.DataType {
		case ort.TensorElementDataTypeInt64:
			data := make([]int64, n)
			t, err := ort.NewTensor(shape, data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "NewTensor: %v\n", err)
				os.Exit(1)
			}
			inVals = append(inVals, t)
		case ort.TensorElementDataTypeInt32:
			data := make([]int32, n)
			t, err := ort.NewTensor(shape, data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "NewTensor: %v\n", err)
				os.Exit(1)
			}
			inVals = append(inVals, t)
		case ort.TensorElementDataTypeFloat:
			data := make([]float32, n)
			t, err := ort.NewTensor(shape, data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "NewTensor: %v\n", err)
				os.Exit(1)
			}
			inVals = append(inVals, t)
		case ort.TensorElementDataTypeDouble:
			data := make([]float64, n)
			t, err := ort.NewTensor(shape, data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "NewTensor: %v\n", err)
				os.Exit(1)
			}
			inVals = append(inVals, t)
		default:
			fmt.Fprintf(os.Stderr, "unsupported dtype for %q\n", meta.Name)
			os.Exit(1)
		}
	}

	outVals := make([]ort.Value, len(outputs))
	defer destroyVals(outVals)

	t0 := time.Now()
	if err := sess.Run(inVals, outVals); err != nil {
		fmt.Fprintf(os.Stderr, "Run: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Run ok in %v (zero-input smoke)\n", time.Since(t0))
	for i, v := range outVals {
		if v == nil {
			continue
		}
		switch t := v.(type) {
		case *ort.Tensor[float32]:
			data := t.GetData()
			fmt.Printf("output[%d] %q float32 len=%d head=%v\n", i, outputs[i].Name, len(data), headF32(data, 8))
		default:
			fmt.Printf("output[%d] %q %T\n", i, outputs[i].Name, v)
		}
	}
	fmt.Println("ok (smoke). Add -tokenizer + -text to use pkg/intentonnx.Route.)")
}

func concreteShapeSmoke(dim ort.Shape, seq int, batch int) (ort.Shape, error) {
	if seq <= 0 {
		return nil, fmt.Errorf("seq must be > 0")
	}
	if batch <= 0 {
		batch = 1
	}
	out := make(ort.Shape, len(dim))
	var unknown []int
	for i, d := range dim {
		if d <= 0 {
			unknown = append(unknown, i)
		}
	}
	if len(dim) == 2 && len(unknown) == 2 {
		out[0] = int64(batch)
		out[1] = int64(seq)
		return out, nil
	}
	for i, d := range dim {
		if d > 0 {
			out[i] = d
			continue
		}
		if len(dim) == 2 && len(unknown) == 1 {
			if unknown[0] == 0 {
				out[i] = int64(batch)
			} else {
				out[i] = int64(seq)
			}
			continue
		}
		out[i] = int64(seq)
	}
	return out, nil
}

func destroyVals(vs []ort.Value) {
	for _, v := range vs {
		if v != nil {
			_ = v.Destroy()
		}
	}
}

func parseLabels(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func headF32(xs []float32, n int) []float32 {
	if len(xs) <= n {
		return append([]float32(nil), xs...)
	}
	return append([]float32(nil), xs[:n]...)
}
