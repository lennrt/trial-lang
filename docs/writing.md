# Write documentation that readers can act on

This repository applies the Plain guidance from
[SimpleEnglish](https://github.com/AminBlg/SimpleEnglish/tree/79b590fc8596523d92c26b1ea7e33236606ef069/skills/simple-english).
The goal is clear prose for readers who do not already know the implementation.
This is a writing method, not a claim of ASD-STE100 certification.

## Preserve the contract

Keep commands, flags, paths, identifiers, quoted errors, and language keywords
exact. Keep normative words such as `MUST` and `SHALL` in requirements. A prose
edit must not silently change an optional behavior into a requirement.

Keep legal terms when they name language features. Define each unfamiliar term
when it first appears. For example, a deposition is a test file with expected
results, and a summons supplies input to a case.

## Make instructions complete

Name the prerequisite and working directory before a command. Give one action
per instruction. Put a condition before the action that depends on it.
For example: "If the program imports a statute, pass its path with `--enact`."

Use short sentences and active voice. Explain what happens to output and state.
State a timeout, size bound, or expected failure when it changes how a reader
runs an example. Keep caveats next to the claim that they limit.

Use the same term for the same concept across pages. Prefer a concrete result
over claims such as "robust" or "seamless." Separate a verified fact from an
estimate or an untested behavior.

## Review the result

Run every new copyable command from the stated directory. Make sure that local
links resolve and that renamed headings still match incoming fragments. Compare
technical statements with the code and the normative files in `spec/`.

Read the longest sentences aloud and split any sentence that carries several
actions. Keep the project theme in examples, but keep operational instructions
direct. Record test evidence and remaining limits in the PR description.
