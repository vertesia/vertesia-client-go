#!/usr/bin/env python3
"""Generate ModelOptions dispatch from the spec; keep generated branch validators."""
import json
from pathlib import Path
import re
import sys

model_path, spec_path = map(Path, sys.argv[1:])
source = model_path.read_text()
schema = json.loads(spec_path.read_text())["components"]["schemas"]["ModelOptions"]
discriminator = schema["discriminator"]
mapping = discriminator["mapping"]
if not mapping or set(mapping.values()) != {branch["$ref"] for branch in schema["oneOf"]}:
    raise ValueError("ModelOptions discriminator must cover every oneOf branch")
# Resolve Go field/type names from generated output instead of duplicating generator naming rules.
struct = re.search(r"type ModelOptions struct \{(.*?)\n\}", source, re.S)
if not struct:
    raise ValueError("Generated ModelOptions struct not found")
fields = dict(re.findall(r"^\s*(\w+)\s+\*(\w+)\s*$", struct[1], re.M))
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
    '\t\treturn fmt.Errorf("unknown or missing ModelOptions discriminator: %q", tag.ID)',
    "\t}",
    "}",
])
source, count = re.subn(
    r"func \(dst \*ModelOptions\) UnmarshalJSON\(data \[\]byte\) error \{.*?\n\}",
    lambda _: "\n".join(lines), source, flags=re.S,
)
if count != 1:
    raise ValueError("Expected exactly one ModelOptions decoder")
source = source.replace('\n\t"gopkg.in/validator.v2"', "")
model_path.write_text(source)
