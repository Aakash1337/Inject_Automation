# `injectctl`

`injectctl` is a local, offline-friendly command-line tool that turns security evidence into draft reports or exercise injects.

In plain terms, you give it things like:
- screenshots
- copied notes
- `nmap` output
- Nessus exports
- simple instructions

It gives you:
- a human-readable Markdown report
- a machine-readable JSON file
- a PDF
- a separate evidence index in Markdown and JSON

It is designed for blue teams, assessment teams, and exercise planners who want a fast first draft without sending data to a cloud service.

## Alpha Status

This project is currently an **alpha**.

That means:
- the core workflow works
- local builds and tests are passing
- the tool is usable today
- the interface and behavior may still change
- you should treat outputs as analyst-reviewed drafts, not final truth

## What It Does

`injectctl` supports two main workflows:

1. `assessment`
   Turn evidence into a draft assessment report with findings, impact, remediation, and evidence links.

2. `inject`
   Turn evidence into a draft inject pack for exercises such as tabletop or blue-team scenarios.

## What You Need

To use the application, you need:

- the `injectctl` binary for your platform
- `Ollama` running locally
- a local model such as `gemma4:26b`
- optionally `gemma4:e4b` as a fallback
- optionally `tesseract` if you want OCR from screenshots

You do **not** need Go installed on the target machine if you are running a released binary.

## Quick Start

### 1. Start Ollama

Make sure Ollama is installed and running.

Then pull the default model:

```powershell
ollama pull gemma4:26b
ollama pull gemma4:e4b
```

### 2. Check Readiness

Run:

```powershell
injectctl doctor
```

You want to see:
- Ollama reachable
- `gemma4:26b` installed
- OCR available if you plan to use screenshots

### 3. Create a Working Folder

Example:

```text
my-run/
  artifacts/
    screenshot.png
    scan.nmap
    notes.txt
  job.yaml
```

Put your evidence into `artifacts/`.

### 4. Generate a Starter Manifest

Assessment example:

```powershell
injectctl init manifest --mode assess --out job.yaml
```

Inject example:

```powershell
injectctl init manifest --mode inject --out job.yaml
```

Then edit `job.yaml` to describe your case.

### 5. Run the Tool

Assessment:

```powershell
injectctl assess run --manifest .\job.yaml --input .\artifacts --out .\out
```

Inject:

```powershell
injectctl inject run --manifest .\job.yaml --input .\artifacts --out .\out
```

### 6. Review the Output

You will usually get:

- `assessment.md` or `inject.md`
- `assessment.json` or `inject.json`
- `assessment.pdf` or `inject.pdf`
- `evidence-index.md`
- `evidence-index.json`

If generation fails, the tool now produces an **evidence-only** result with an explicit error report instead of pretending a polished draft was created.

## Example Manifest

Assessment example:

```yaml
mode: assess
title: Example Assessment
client: Example Corp
environment: Production
classification: TLP:AMBER
instructions: >
  Turn the supplied screenshots, scan outputs, and notes into a draft corporate assessment report.
template: ./templates/default/assessment.md.tmpl
artifacts:
  - ./artifacts
ai:
  provider: ollama
  endpoint: http://127.0.0.1:11434
  model: gemma4:26b
  fallback_model: gemma4:e4b
  temperature: 0.2
  max_tokens: 2048
  timeout_seconds: 90
  max_prompt_artifacts: 12
  max_prompt_observations: 40
  # prompt_dir: ./prompts
output:
  formats:
    - markdown
    - json
    - pdf
  project_dir: ./project
  redaction_rules:
    - '(?m)\b\d{1,3}(?:\.\d{1,3}){3}\b'
```

More examples:

- [examples/assessment-manifest.yaml](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/examples/assessment-manifest.yaml)
- [examples/inject-manifest.yaml](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/examples/inject-manifest.yaml)

## Plain-English Manifest Field Guide

- `mode`
  Choose `assess` or `inject`.

- `title`
  The name shown in the final output.

- `client`
  The organization or customer name.

- `environment`
  The environment being assessed or exercised, such as `Production`.

- `classification`
  Your handling label, such as `TLP:AMBER`.

- `instructions`
  Tell the model what you want. Keep this simple and direct.

- `template`
  Optional Markdown template file for the final Markdown output.

- `artifacts`
  One or more files or folders containing the evidence.

- `ai.endpoint`
  Usually `http://127.0.0.1:11434` for local Ollama.

- `ai.model`
  Main Ollama model. Default is `gemma4:26b`.

- `ai.fallback_model`
  Backup model if the primary model fails.

- `ai.timeout_seconds`
  How long the tool waits for Ollama before timing out a request.

- `ai.max_prompt_artifacts`
  Limits how many artifacts are sent into the AI prompt context.

- `ai.max_prompt_observations`
  Limits how many normalized observations are sent into the AI prompt context.

- `ai.prompt_dir`
  Optional folder containing custom AI prompt files such as `assessment_system.txt` and `inject_system.txt`.

- `output.formats`
  Which outputs to create: `markdown`, `json`, `pdf`.

- `output.project_dir`
  Optional folder where the tool stores a local project copy of the run.

- `output.redaction_rules`
  Optional regex or literal strings to redact from outputs.

## Supported Evidence Today

Current built-in parsing supports:

- screenshots and images
- plain text notes
- `nmap` text output
- `nmap` XML
- Nessus CSV
- Nessus `.nessus` XML

Images are handled like this:

1. OCR is attempted first if `tesseract` is installed.
2. Images are also passed to Ollama for multimodal understanding when possible.

## AI Behavior

The AI layer now works like this:

1. `injectctl` checks Ollama availability.
2. It checks whether the primary model is installed.
3. If the primary model is missing and the fallback model exists, it switches to the fallback automatically.
4. Prompt input is trimmed to configurable artifact and observation limits so very large runs do not overload the prompt.
5. Prompt system instructions come from embedded defaults unless you provide a custom `prompt_dir`.
6. The model must return structured JSON.
7. If the JSON is malformed, the tool retries once with a repair prompt.
8. If synthesis still fails, the tool emits an `evidence_only` result with an error report.

## Commands

### `injectctl doctor`

Checks whether your local environment is ready.

Example:

```powershell
injectctl doctor
```

Optional flags:

```powershell
injectctl doctor --endpoint http://127.0.0.1:11434 --model gemma4:26b --fallback-model gemma4:e4b
```

### `injectctl init manifest`

Creates a starter manifest.

Example:

```powershell
injectctl init manifest --mode assess --out job.yaml
```

### `injectctl assess run`

Builds a draft assessment report.

Example:

```powershell
injectctl assess run --manifest .\job.yaml --input .\artifacts --out .\out
```

### `injectctl inject run`

Builds a draft inject pack.

Example:

```powershell
injectctl inject run --manifest .\job.yaml --input .\artifacts --out .\out
```

### `injectctl template validate`

Checks that a custom Markdown template can be parsed.

Example:

```powershell
injectctl template validate --template .\templates\custom-report.md.tmpl
```

## Template Notes

Templates use Go's standard text template syntax.

That means placeholders look like:

```gotemplate
{{ .Config.Title }}
{{ .Draft.ExecutiveSummary }}
```

The built-in templates are here:

- [templates/default/assessment.md.tmpl](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/default/assessment.md.tmpl)
- [templates/default/inject.md.tmpl](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/default/inject.md.tmpl)

Important:
- custom templates affect Markdown output
- PDF output uses the built-in PDF layout

Custom AI system prompts are separate from report templates.

If you set `ai.prompt_dir`, the tool will look for:

- `assessment_system.txt`
- `inject_system.txt`

## Output Files

### Markdown

Best for:
- human review
- editing
- turning into internal reports

### JSON

Best for:
- automation
- pipelines
- storing normalized findings or injects
- reviewing detailed AI/error state

### PDF

Best for:
- sharing with non-technical stakeholders
- executive review
- preserving layout

### Evidence Index

Best for:
- auditing exactly which artifacts were processed
- checking which observations were linked to each artifact
- reviewing snippets without reading the full report

## Safety and Review Expectations

This tool is meant to help analysts, not replace them.

You should always:
- review findings before distribution
- verify remediation text
- confirm evidence links
- check severity and impact wording

## Troubleshooting

### `doctor` says Ollama is not reachable

Check that Ollama is running and listening on `http://127.0.0.1:11434`.

### `doctor` says model is missing

Pull the model:

```powershell
ollama pull gemma4:26b
```

### Screenshots are not extracting text

Install `tesseract` so OCR can run locally.

### Output says `evidence_only`

This means the tool collected and normalized the evidence, but model generation failed. Review the JSON output and the error report, fix the local model/runtime issue, and run again.

### Redaction did not behave as expected

Check your `redaction_rules`. They can be regex patterns or literal strings. Invalid regex patterns are treated as literal text and added to warnings.

## Building From Source

If you want to build the binary yourself, you need Go installed.

Test the project:

```powershell
go test ./...
```

Build the local binary:

```powershell
go build ./cmd/injectctl
```

## Release Builds

Use the release script to build all supported targets:

```powershell
.\scripts\release.ps1 -Version v0.1.0
```

Artifacts are written under `dist/<version>/`.

Current release targets:

- `windows-amd64`
- `linux-amd64`
- `linux-arm64`
- `darwin-amd64`
- `darwin-arm64`

The release script also creates:

- zipped binaries
- `sha256sums.txt`

## Current Limitations

- still alpha
- no live multi-user server mode
- no embedded local model runtime
- OCR depends on an external local install of `tesseract`
- PDF templates are not user-customizable yet
- outputs should still be reviewed by a human analyst

## Repository Layout

- [cmd/injectctl](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/cmd/injectctl)
- [internal](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/internal)
- [examples](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/examples)
- [templates/default](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/default)
- [schemas/job.schema.json](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/schemas/job.schema.json)
- [scripts/release.ps1](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/scripts/release.ps1)
