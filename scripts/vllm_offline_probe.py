#!/usr/bin/env python3
import argparse
import json
import sys
import time


def token_count(value):
    if value is None:
        return 0
    try:
        return len(value)
    except TypeError:
        return 0


def render_chat_prompt(llm, prompt):
    try:
        tokenizer = llm.get_tokenizer()
    except AttributeError:
        return prompt

    if tokenizer is None or not hasattr(tokenizer, "apply_chat_template"):
        return prompt

    try:
        return tokenizer.apply_chat_template(
            [{"role": "user", "content": prompt}],
            tokenize=False,
            add_generation_prompt=True,
        )
    except Exception:
        return prompt


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--model", required=True)
    parser.add_argument("--prompt", required=True)
    parser.add_argument("--max-tokens", type=int, default=128)
    args = parser.parse_args()

    try:
        from vllm import LLM, SamplingParams
    except ImportError:
        print("vLLM Python package is required for offline mode", file=sys.stderr)
        return 127

    started = time.perf_counter()
    llm = LLM(model=args.model)
    loaded = time.perf_counter()

    prompt = render_chat_prompt(llm, args.prompt)
    outputs = llm.generate([prompt], SamplingParams(max_tokens=args.max_tokens))
    generated = time.perf_counter()

    request_output = outputs[0]
    completion = request_output.outputs[0]
    payload = {
        "content": completion.text,
        "load_ms": round((loaded - started) * 1000),
        "generate_ms": round((generated - loaded) * 1000),
        "total_ms": round((generated - started) * 1000),
        "prompt_tokens": token_count(getattr(request_output, "prompt_token_ids", None)),
        "generated_tokens": token_count(getattr(completion, "token_ids", None)),
    }
    print(json.dumps(payload, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
