from __future__ import annotations

import argparse
from pathlib import Path
import re

from docx import Document


def main() -> int:
    parser = argparse.ArgumentParser(description="Convert simple DOCX templates into Markdown.")
    parser.add_argument("inputs", nargs="+", help="DOCX files or directories to convert")
    parser.add_argument("--out-dir", default="", help="Output directory for converted Markdown files")
    args = parser.parse_args()

    paths = collect_inputs(args.inputs)
    if not paths:
        raise SystemExit("No .docx files found.")

    out_dir = Path(args.out_dir) if args.out_dir else None
    if out_dir is not None:
        out_dir.mkdir(parents=True, exist_ok=True)

    for path in paths:
        markdown = convert_document(path)
        destination = build_destination(path, out_dir)
        destination.write_text(markdown, encoding="utf-8")
        print(destination)
    return 0


def collect_inputs(values: list[str]) -> list[Path]:
    discovered: list[Path] = []
    for value in values:
        path = Path(value)
        if path.is_dir():
            discovered.extend(sorted(path.glob("*.docx")))
            continue
        if path.suffix.lower() == ".docx" and path.exists():
            discovered.append(path)
    return discovered


def build_destination(path: Path, out_dir: Path | None) -> Path:
    slug = slugify(path.stem)
    name = f"{slug}.md"
    if out_dir is None:
        return path.with_suffix(".md")
    return out_dir / name


def convert_document(path: Path) -> str:
    doc = Document(path)
    lines = [
        f"<!-- Auto-converted from {path.name}. Review before using as a live template. -->",
        "",
    ]

    for paragraph in doc.paragraphs:
        text = normalize_text(paragraph.text)
        if not text:
            if lines and lines[-1] != "":
                lines.append("")
            continue

        lines.extend(render_paragraph(text, paragraph.style.name))

    return "\n".join(collapse_blank_lines(lines)).strip() + "\n"


def render_paragraph(text: str, style_name: str) -> list[str]:
    style = (style_name or "").lower()

    if style.startswith("heading"):
        level = extract_heading_level(style_name)
        return [f"{'#' * level} {text}", ""]

    if re.fullmatch(r"heading\s+\d+", text.strip(), flags=re.IGNORECASE):
        number = int(re.search(r"(\d+)", text).group(1))
        level = min(max(number + 1, 2), 6)
        return [f"{'#' * level} {text}", ""]

    if text == "<picture>":
        return ["![Image Placeholder](./image-placeholder.png)", ""]

    if text.lower().startswith("figure #:"):
        return [f"*{text}*", ""]

    if ":" in text and text.split(":", 1)[0].strip().lower() in {"team", "inject name", "inject id"}:
        label, value = text.split(":", 1)
        return [f"**{label.strip()}:** {value.strip()}", ""]

    if text.isupper() and len(text.split()) <= 8:
        return [f"# {text.title()}", ""]

    return [text, ""]


def extract_heading_level(style_name: str) -> int:
    match = re.search(r"(\d+)", style_name)
    if not match:
        return 2
    level = int(match.group(1))
    return min(max(level, 1), 6)


def normalize_text(text: str) -> str:
    return " ".join(text.replace("\xa0", " ").split())


def collapse_blank_lines(lines: list[str]) -> list[str]:
    collapsed: list[str] = []
    blank = False
    for line in lines:
        if line == "":
            if blank:
                continue
            blank = True
            collapsed.append(line)
            continue
        blank = False
        collapsed.append(line)
    return collapsed


def slugify(value: str) -> str:
    value = value.lower()
    value = re.sub(r"[^a-z0-9]+", "-", value)
    value = value.strip("-")
    return value or "template"


if __name__ == "__main__":
    raise SystemExit(main())
