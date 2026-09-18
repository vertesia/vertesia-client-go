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
if '"strings"' not in source:
    source = source.replace('"encoding/json"', '"encoding/json"\n\t"strings"', 1)
if "func modelOptionsIsKnownField(" not in source:
    source += '''
// Match encoding/json's case-insensitive field lookup too, so an alternate
// spelling cannot resurrect a cleared typed field during a later round trip.
func modelOptionsIsKnownField(name string, known []string) bool {
    for _, field := range known {
        if strings.EqualFold(name, field) { return true }
    }
    return false
}
'''
model_path.write_text(source)

# Keep extensions on each concrete subtype, so extracting and rewrapping typed
# options does not lose them. Retain the generator's validation and ToMap logic.
model_files = {}
for path in model_path.parent.glob("model_*.go"):
    content = path.read_text()
    for name in re.findall(r"^type (\w+) struct \{", content, re.M):
        model_files[name] = path
for ref in mapping.values():
    name = ref.removeprefix("#/components/schemas/")
    path = model_files[fields[name]]
    content = path.read_text()
    name = fields[name]
    # Required-field checks only need key presence, not float64 conversion of
    # unrelated extension values (which can contain arbitrary JSON numbers).
    content = content.replace("allProperties := make(map[string]interface{})", "allProperties := make(map[string]json.RawMessage)")
    struct = re.search(r"type " + name + r" struct \{(.*?)\n\}", content, re.S)
    # Read the actual generated wire fields rather than guessing Go field names.
    known = re.findall(r'`json:"([^",]+)(?:,[^"]*)?"`', struct[1])
    known = [field for field in known if field != "-"]
    if not known:
        raise ValueError(f"No generated wire fields for {name}")
    known_literal = "[]string{" + ", ".join(json.dumps(field) for field in known) + "}"
    if "AdditionalProperties" not in struct[1]:
        content = content.replace(
            f"type {name} struct {{",
            f"type {name} struct {{\n"
            '\t// AdditionalProperties preserves unknown option fields across read-edit-save.\n'
            '\tAdditionalProperties map[string]interface{} `json:"-"`', 1,
        )
    else:
        content, count = re.subn(
            r'AdditionalProperties\s+map\[string\]interface\{\}(?:\s+`json:"-"`)?',
            'AdditionalProperties map[string]interface{} `json:"-"`', content, count=1,
        )
        if count != 1:
            raise ValueError(f"Unexpected generated AdditionalProperties representation for {name}")
    # Some older branches already had generator-owned extension handling. Replace
    # it with the precision-preserving wrapper below, retaining the public map API.
    content = re.sub(
        r'\n\s*additionalProperties := make\(map\[string\]interface\{\}\).*?\n\t\}',
        '', content, flags=re.S,
    )
    content = re.sub(
        r'\n\s*for key, value := range o.AdditionalProperties \{\s*toSerialize\[key\] = value\s*\}',
        '', content,
    )
    if f"func (o *{name}) unmarshalKnownJSON(" not in content:
        content, count = re.subn(
            r"func \(o \*" + name + r"\) UnmarshalJSON\(",
            f"func (o *{name}) unmarshalKnownJSON(", content,
        )
        if count == 0:
            content += f'''
func (o *{name}) unmarshalKnownJSON(data []byte) error {{
    type plain {name}
    return json.Unmarshal(data, (*plain)(o))
}}
'''
        elif count != 1:
            raise ValueError(f"Unexpected decoder count for {name}")
    wrapper = f'''func (o *{name}) UnmarshalJSON(data []byte) error {{
    *o = {name}{{}}
    var decoded {name}
    if err := decoded.unmarshalKnownJSON(data); err != nil {{ return err }}
    var extra map[string]json.RawMessage
    if err := json.Unmarshal(data, &extra); err != nil {{ return err }}
    for key := range extra {{
        if modelOptionsIsKnownField(key, {known_literal}) {{ delete(extra, key) }}
    }}
    if len(extra) > 0 {{
        decoded.AdditionalProperties = make(map[string]interface{{}}, len(extra))
        for key, value := range extra {{ decoded.AdditionalProperties[key] = value }}
    }}
    *o = decoded
    return nil
}}'''
    content, count = re.subn(
        r"func \(o \*" + name + r"\) UnmarshalJSON\([^\n]*\{.*?\n\}",
        lambda _: wrapper, content, flags=re.S,
    )
    if count == 0:
        content += "\n" + wrapper + "\n"
    elif count != 1:
        raise ValueError(f"Unexpected wrapper count for {name}")
    if "// Preserve only unknown fields; typed fields retain precedence even when cleared." not in content:
        content, count = re.subn(
            r"(func \(o " + name + r"\) ToMap\(\) \(map\[string\]interface\{\}, error\) \{\s*"
            r"toSerialize := map\[string\]interface\{\}\{\})",
            lambda match: match[1] + f'''
    // Preserve only unknown fields; typed fields retain precedence even when cleared.
    for key, value := range o.AdditionalProperties {{
        if !modelOptionsIsKnownField(key, {known_literal}) {{ toSerialize[key] = value }}
    }}''', content,
        )
        if count != 1:
            raise ValueError(f"Expected generated ToMap for {name}")
    path.write_text(content)
