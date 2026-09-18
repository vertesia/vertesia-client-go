#!/usr/bin/env python3
"""Decode known ModelOptions by discriminator; preserve legacy/future payloads."""
import json
from pathlib import Path
import re
import sys

model_path, spec_path = map(Path, sys.argv[1:])
source = model_path.read_text()
components = json.loads(spec_path.read_text())["components"]["schemas"]
schema = components["ModelOptions"]
branches = schema.get("oneOf", schema.get("anyOf", []))
discriminator = schema.get("discriminator", {"propertyName": "_option_id"})
mapping = {}
for branch in branches:
    ref = branch["$ref"]
    member = components[ref.removeprefix("#/components/schemas/")]
    tag = member["properties"][discriminator["propertyName"]]
    values = tag.get("enum", [tag.get("const")])
    if len(values) != 1 or not isinstance(values[0], str) or values[0] in mapping:
        raise ValueError(f"Expected a unique literal option ID in {ref}")
    mapping[values[0]] = ref
if not mapping or ("mapping" in discriminator and discriminator["mapping"] != mapping):
    raise ValueError("ModelOptions family IDs must cover every union branch")
# Resolve Go field/type names from generated output instead of duplicating generator naming rules.
struct = re.search(r"type ModelOptions struct \{(.*?)\n\}", source, re.S)
if not struct:
    raise ValueError("Generated ModelOptions struct not found")
fields = dict(re.findall(r"^\s*(\w+)\s+\*(\w+)\s*$", struct[1], re.M))
if not re.search(r"^\s*Raw\s+json.RawMessage\b", struct[1], re.M):
    source = source.replace(
        "type ModelOptions struct {",
        'type ModelOptions struct {\n'
        '\t// Raw preserves options with a missing or unknown discriminator without guessing a subtype.\n'
        '\tRaw json.RawMessage `json:"-"`',
        1,
    )
lines = [
    "func (dst *ModelOptions) UnmarshalJSON(data []byte) error {",
    "\t*dst = ModelOptions{}",
    "\tvar tag struct {",
    f'\t\tID string `json:"{discriminator["propertyName"]}"`',
    "\t}",
    "\tif err := json.Unmarshal(data, &tag); err != nil { return err }",
    "\tswitch tag.ID {",
]
for value, ref in mapping.items():
    name = ref.removeprefix("#/components/schemas/")
    if name not in fields:
        raise ValueError(f"No generated ModelOptions field for {ref}")
    lines.extend([
        f"\tcase {json.dumps(value)}:",
        f"\t\tvar branch {fields[name]}",
        "\t\tif err := json.Unmarshal(data, &branch); err != nil { return err }",
        f"\t\tdst.{name} = &branch",
        "\t\treturn nil",
    ])
lines.extend([
    "\tdefault:",
    "\t\treturn dst.Raw.UnmarshalJSON(data)",
    "\t}",
    "}",
])

def replace_method(name, body, allow_missing=False):
    global source
    source, count = re.subn(
        r"func \((?:dst \*|src |obj \*?)ModelOptions\) " + name + r"\([^\n]*\{.*?\n\}",
        lambda _: "\n".join(body), source, flags=re.S,
    )
    if count == 0 and allow_missing:
        source += "\n\n" + "\n".join(body) + "\n"
    elif count != 1:
        raise ValueError(f"Expected exactly one ModelOptions {name} method")


replace_method("UnmarshalJSON", lines)
# Preserve generated branch precedence, including when callers set a typed branch
# after reading an unrecognized payload. Raw is only used when no typed branch is set.
for name, receiver, returns, expression, fallback in [
    ("MarshalJSON", "src ModelOptions", "([]byte, error)", "json.Marshal(src.{field})", "json.Marshal(src.Raw)"),
    ("GetActualInstance", "obj *ModelOptions", "interface{}", "obj.{field}", "obj.Raw"),
    ("GetActualInstanceValue", "obj ModelOptions", "interface{}", "*obj.{field}", "obj.Raw"),
]:
    variable = receiver.split()[0]
    body = [f"func ({receiver}) {name}() {returns} {{"]
    if name == "GetActualInstance":
        body.append("\tif obj == nil { return nil }")
    for field in fields:
        body.extend([
            f"\tif {variable}.{field} != nil {{",
            f"\t\treturn {expression.format(field=field)}",
            "\t}",
        ])
    if name != "MarshalJSON":
        body.append(f"\tif {variable}.Raw == nil {{ return nil }}")
    body.extend([f"\treturn {fallback}", "}"])
    replace_method(name, body, allow_missing="anyOf" in schema and name != "MarshalJSON")
# The generator omits oneOf convenience APIs for anyOf. Keep those existing
# constructors available when the schema relaxes the ID requirement.
for field, field_type in fields.items():
    constructor = f"{field}AsModelOptions"
    if not re.search(r"func " + re.escape(constructor) + r"\(", source):
        source += (
            f"\n// {constructor} wraps typed options in ModelOptions.\n"
            f"func {constructor}(v *{field_type}) ModelOptions {{\n"
            f"\treturn ModelOptions{{{field}: v}}\n"
            "}\n"
        )
source = source.replace('\n\t"gopkg.in/validator.v2"', "").replace('\n\t"fmt"', "")
model_path.write_text(source)
