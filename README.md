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

## V1 Status

This project is currently a **v1 release candidate**.

That means:
- the core workflow works
- local builds and tests are passing
- the tool is usable today
- the interface should now be mostly stable
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
- at least one local Gemma 4 model profile
- `gemma4:26b` for the default `balanced` profile
- `gemma4:e4b` as the normal fallback
- optionally `gemma4:31b` for the `quality` profile
- optionally `tesseract` if you want OCR from screenshots

You do **not** need Go installed on the target machine if you are running a released binary.

## Install Dependencies

This project depends on two external local tools:

- `Ollama` for local AI inference
- `Tesseract` for OCR from screenshots and images

If `Ollama` is missing, the run will fail.
If `Tesseract` is missing, image OCR will be reduced or unavailable depending on the artifact and model path.

### Install Ollama

Official download:

- [Ollama Download](https://ollama.com/download)

#### Windows

1. Open the Ollama download page.
2. Download the Windows installer.
3. Run the installer.
4. After install, start Ollama from the Start menu or let the installer start it automatically.
5. Open a new PowerShell window and verify the CLI is available:

```powershell
ollama --version
```

6. Verify the local service responds:

```powershell
ollama list
```

If Ollama is running correctly, `ollama list` should return a model table or an empty list instead of a connection failure.

#### macOS

1. Download the macOS installer from the Ollama download page.
2. Install and launch Ollama.
3. Verify:

```bash
ollama --version
ollama list
```

#### Linux

Use the install instructions from the Ollama download page, then verify:

```bash
ollama --version
ollama list
```

### Pull the Required Gemma 4 Models

After Ollama is installed and running, pull the models used by `injectctl`:

```powershell
ollama pull gemma4:26b
ollama pull gemma4:e4b
ollama pull gemma4:31b
```

Verify they are present:

```powershell
ollama list
```

You should see:

- `gemma4:26b`
- `gemma4:e4b`
- `gemma4:31b`

### Install Tesseract

Official installation reference:

- [Tesseract Installation Guide](https://tesseract-ocr.github.io/tessdoc/Installation.html)

For Windows, the Tesseract project documentation currently points users to the UB Mannheim Windows installers:

- [Tesseract at UB Mannheim](https://github.com/UB-Mannheim/tesseract/wiki)

#### Windows

1. Open the UB Mannheim Tesseract page.
2. Download the current Windows installer.
3. Run the installer.
4. Install to the default path unless you have a reason to change it:

```text
C:\Program Files\Tesseract-OCR
```

5. Make sure that folder is in your `PATH`.

To check from PowerShell:

```powershell
tesseract --version
```

If PowerShell says `tesseract` is not recognized, add this directory to your user or system `PATH` and open a new shell:

```text
C:\Program Files\Tesseract-OCR
```

#### macOS

The Tesseract installation guide documents Homebrew and MacPorts. Homebrew is usually simplest:

```bash
brew install tesseract
tesseract --version
```

#### Linux

The Tesseract installation guide documents package-manager installation by distribution. On Ubuntu:

```bash
sudo apt install tesseract-ocr
tesseract --version
```

### Final Verification

Once Ollama and Tesseract are installed, verify the full local environment:

```powershell
injectctl doctor
```

For a stronger validation, run a live smoke test against your configured model:

```powershell
injectctl doctor --profile balanced --smoke
```

You want to see:

- Ollama reachable
- expected model profile resolved
- installed models listed
- smoke test passed
- OCR available

## Quick Start

### 1. Start Ollama

Make sure Ollama is installed and running.

Then pull the default model:

```powershell
ollama pull gemma4:26b
ollama pull gemma4:e4b
```

Optional quality profile:

```powershell
ollama pull gemma4:31b
```

### 2. Check Readiness

Run:

```powershell
injectctl doctor
```

You want to see:
- Ollama reachable
- the selected profile resolved to installed models
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

If you want a complete working example bundle instead of starting from scratch:

```powershell
injectctl init demo --mode assess --out .\demo
```

That creates:
- a ready-to-run `job.yaml`
- sample `notes.txt`
- sample `scan.nmap`
- a sample screenshot `terminal.png`

### 5. Run the Tool

Assessment:

```powershell
injectctl assess run --manifest .\job.yaml --input .\artifacts --out .\out
```

Inject:

```powershell
injectctl inject run --manifest .\job.yaml --input .\artifacts --out .\out
```

After a successful run, `injectctl` now prints a short completion summary with the status, output directory, project snapshot directory, AI model usage, and generated file names.

### 6. Review the Output

You will usually get:

- `assessment-report-draft.md` or `inject-pack-draft.md`
- `assessment-report-data.json` or `inject-pack-data.json`
- `assessment-report-review.pdf` or `inject-pack-review.pdf`
- `assessment-evidence-index.md` or `inject-evidence-index.md`
- `assessment-evidence-index.json` or `inject-evidence-index.json`

The default Markdown and JSON outputs now also record the AI models actually used during the run, plus batch counts when oversized evidence had to be split across multiple synthesis passes.

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
# template: ./templates/default/assessment.md.j2
# template_dir: ./templates/library
artifacts:
  - ./artifacts
ai:
  provider: ollama
  endpoint: http://127.0.0.1:11434
  profile: balanced
  # model: gemma4:26b
  # fallback_model: gemma4:e4b
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

- `template_dir`
  Optional folder containing a collection of templates. If `template` is not set, `injectctl` will score the available templates and auto-select the best match for the current mode and instructions.

- `artifacts`
  One or more files or folders containing the evidence.

- `ai.endpoint`
  Usually `http://127.0.0.1:11434` for local Ollama.

- `ai.profile`
  Simple model preset. Use `fast`, `balanced`, or `quality`.

- `ai.model`
  Optional explicit primary model override. If omitted, the selected profile sets it.

- `ai.fallback_model`
  Optional explicit fallback override. If omitted, the selected profile sets it.

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
2. It resolves an AI profile such as `balanced` or `quality` into primary and fallback models.
3. It checks whether the primary model is installed.
4. If the primary model is missing and the fallback model exists, it switches to the fallback automatically.
5. Prompt input is still bounded by configurable artifact and observation limits, but oversized evidence sets are now split into multiple AI batches and consolidated into one final draft.
6. Prompt system instructions come from embedded defaults unless you provide a custom `prompt_dir`.
7. The model must return structured JSON.
8. If the JSON is malformed, the tool retries once with a repair prompt.
9. If synthesis still fails, the tool emits an `evidence_only` result with an error report.

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

Profile-based example:

```powershell
injectctl doctor --profile quality --smoke
```

### `injectctl init manifest`

Creates a starter manifest.

Example:

```powershell
injectctl init manifest --mode assess --out job.yaml
```

### `injectctl init demo`

Creates a full demo folder with sample artifacts and a starter manifest.

Example:

```powershell
injectctl init demo --mode assess --out .\demo
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
injectctl template validate --template .\templates\custom-report.md.j2
```

### `injectctl template list`

Lists candidate templates from a directory in the order they would be considered.

Example:

```powershell
injectctl template list --dir .\templates\library --mode assess
```

## Template Notes

Templates support Jinja-style Markdown templates.

Legacy Go `text/template` files with extensions like `.tmpl` are still accepted for backward compatibility, but `.j2` or `.jinja` is the recommended v1 format.

That means placeholders look like:

```jinja
{{ config.title }}
{{ draft.executive_summary }}
{% if draft.findings %}
{{ draft.findings|length }} findings
{% endif %}
```

The built-in templates are here:

- [templates/default/assessment.md.j2](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/default/assessment.md.j2)
- [templates/default/inject.md.j2](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/default/inject.md.j2)

The shipped template library is here:

- [templates/library/assessment-ir-report.md.j2](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/library/assessment-ir-report.md.j2)
- [templates/library/inject-business-brief.md.j2](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/library/inject-business-brief.md.j2)
- [templates/library/inject-legal-memo.md.j2](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/library/inject-legal-memo.md.j2)
- [templates/library/inject-technical-worksheet.md.j2](/C:/Users/blis/Desktop/Projects/Blue%20Team/Inject%20Automation/templates/library/inject-technical-worksheet.md.j2)

Selection rules:

- if `template` is set, that file is used
- if `template` is not set and `template_dir` is set, `injectctl` scores the directory contents and auto-selects the best match
- if nothing in `template_dir` is compatible, the built-in default template is used

You can keep converted or source material in the same broader `templates/` tree, but only text-based template files are selectable:

- `.j2`
- `.jinja`
- `.jinja2`
- `.tmpl`
- `.tpl`
- `.md`

Direct `.docx` rendering is not supported. If you want to reuse Word templates, convert them first:

```powershell
python .\scripts\docx_to_markdown.py .\templates --out-dir .\templates\converted-docx
```

Those converted Markdown files are source material for building live templates. The ready-to-use shipped live templates are in `templates/library/`.

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

### Project Snapshot

If `output.project_dir` is set, the project folder now also keeps:
- the manifest snapshot
- the full run JSON
- `evidence-index.json`
- `evidence-index.md`
- `run-summary.json`
- `output-inventory.json`
- `run-log.jsonl`
- `outputs/` with copied Markdown, JSON, and PDF artifacts for that run

`run-summary.json` includes the configured model, the actual models used, stage-level AI execution details, and the batch count for chunked synthesis runs.
`output-inventory.json` records the generated output files, their paths, sizes, and timestamps.
`run-log.jsonl` records structured lifecycle events for the run so you can audit what happened without reading console output.

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

Start by verifying the local CLI and service:

```powershell
ollama --version
ollama list
```

If `ollama list` fails, launch Ollama first and then rerun `injectctl doctor`.

### `doctor` says model is missing

Pull the model:

```powershell
ollama pull gemma4:26b
```

### Screenshots are not extracting text

Install `tesseract` so OCR can run locally.

Verify it is available:

```powershell
tesseract --version
```

If the command is not found on Windows, add this to your `PATH` and open a new shell:

```text
C:\Program Files\Tesseract-OCR
```

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
- `windows-arm64`
- `linux-amd64`
- `linux-arm64`
- `darwin-amd64`
- `darwin-arm64`

The release script also creates:

- zipped binaries
- `sha256sums.txt`

GitHub Actions also builds and uploads per-platform artifacts for each run.

## Current Limitations

- still analyst-review-first, not autopublish
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
