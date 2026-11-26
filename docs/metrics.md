[Documentation](README.md) › Metrics

# Metrics

bashq measures every function of a script, and the code outside functions
as a pseudo-function named `(top level)`. All measures but line width come
from the syntax tree, so text in strings, heredocs and comments never
counts. Neither do the awk, jq or SQL programs a script passes as arguments.

| Measure | Of | Rule that uses it |
| --- | --- | --- |
| [Cognitive complexity](#cognitive-complexity) | function | [`cognitive-complexity`](rules.md#cognitive-complexity) |
| [Cyclomatic complexity](#cyclomatic-complexity) | function | [`cyclomatic-complexity`](rules.md#cyclomatic-complexity) |
| [Length](#length) | function, file | [`function-length`](rules.md#function-length), [`file-length`](rules.md#file-length) |
| [Nesting depth](#nesting-depth) | function | [`nesting-depth`](rules.md#nesting-depth) |
| [Line width](#line-width) | line | [`line-length`](rules.md#line-length) |
| [Lines of code and comments](#lines-of-code-and-comments) | file | none (reported by `bashq metrics`) |
| [Duplicated lines](#duplicated-lines) | file | [`duplication`](rules.md#duplication) |

## Functions

Every form of declaration counts: `f() { … }`, `function f { … }`,
`function f() { … }`, `f() ( … )` (a subshell body). A name may hold `::`,
`.` or `-` (`logger::info`).

A function declared inside another one is measured on its own, from nesting
0, and its code does not count in the function around it.

## Cognitive complexity

[Cognitive complexity](https://www.sonarsource.com/resources/cognitive-complexity/)
is SonarSource's measure of how hard code is to read. bashq follows the
specification's three rules and adapts them to bash:

1. **Breaks in the linear flow cost 1**: `if`, `elif`, `else`, loops,
   `case`, and sequences of logical operators.
2. **Nesting costs more**: a structure inside another one costs 1 plus its
   nesting level.
3. **Shorthand is free**: what makes code shorter without making it harder
   to read costs nothing, such as error handling with `cmd || return 1`.

### Increments

| Construct | Cost | Raises the nesting of its body |
| --- | --- | --- |
| `if` | 1 + nesting | yes |
| `elif`, `else` | 1 | yes |
| `for`, `while`, `until`, `select` (and `for ((…))`) | 1 + nesting | yes |
| `case` (once, whatever the number of branches) | 1 + nesting | yes |
| a list that starts with a test: `[[ … ]] && cmd`, `[ … ] \|\| cmd`, `(( … )) && cmd` | 1 + nesting | yes: the commands after the test |
| a sequence of like operators in a list, a `[[ ]]` or a `(( ))`: `a && b && c` | 1 | no |
| each change of operator: `a && b \|\| c` costs 2 | 1 | no |
| ternary `$(( a ? b : c ))` | 1 + nesting | no |
| `break N`, `continue N` (N > 1): a jump out of several loops | 1 | no |
| recursion: the function calls itself | 1 | no |

What is free:

- straight-line commands, pipelines, assignments, `local`, redirections;
- **error handling**: a command followed by `|| return`, `|| exit`,
  `|| break`, `|| continue`, or `|| { …; return 1; }`. The operator before
  it costs nothing: `cd "${dir}" || return 1` is 0, while
  `cd "${dir}" && make || return 1` is 1, for the `&&`;
- `break`, `continue` (one loop), `return`, `exit`;
- `{ … }` groups and `( … )` subshells: they neither cost nor nest;
- parameter expansion defaults (`${x:-default}`), like `??` in Sonar's
  specification.

### Bash-specific choices

**A test followed by `&&` or `||` is an `if`.** `[[ -n $x ]] && echo "$x"`
reads as "if x is set, print it". It costs what the `if` would, nesting
included, and the commands after the test are nested one level deeper. The
same-operator commands that follow belong to it: `[[ -d $d ]] && cd "$d" &&
make` is one `if`.

**`[[ -n $1 ]] || return 1` is an `if` too.** A test is a decision, whatever
the command after it. The free `|| return` is reserved for a *command* that
fails (error handling): `mkdir -p "$d" || return 1` is 0.

**Conditions are the structure's own.** In `if [[ -n $a ]] && [[ -n $b ]];
then`, the tests belong to the `if`. Only the `&&` costs 1, as a sequence of
operators: the condition is not counted as a hidden `if`.

**Code in `$( … )`, `<( … )` and `$(( … ))` counts where it is.** An `if`
in a command substitution inside a loop pays the loop's nesting.

### Reading a score

`bashq explain` prints the function with each increment in the margin:

```console
$ bashq explain examples/deploy/deploy.sh:deploy::main
deploy::main examples/deploy/deploy.sh:12: cognitive complexity 17, cyclomatic 9, 32 lines, nesting 4

  12     │ deploy::main() {
  13     │   local dry_run=false
  14 +1  │   if [[ "${1:-}" == "-n" ]]; then  # +1 if
  15     │     dry_run=true
  16     │     shift
  17     │   fi
  18 +1  │   [[ $# -ge 2 ]] || { deploy::usage; return 2; }  # +1 conditional
  19     │   local version="$1"
  20     │   shift
  21     │
  22 +1  │   for host in "$@"; do  # +1 for
  23 +2  │     if ! ssh "${host}" true; then  # +2 if (nesting 1)
  24     │       echo "skip ${host}: unreachable" >&2
  25     │       continue
  26     │     fi
  27 +2  │     if [[ "${dry_run}" == true ]]; then  # +2 if (nesting 1)
  28     │       echo "would deploy ${version} to ${host}"
  29 +1  │     else  # +1 else
  30 +3  │       for attempt in 1 2 3; do  # +3 for (nesting 2)
  31 +4  │         if scp "app-${version}.tar.gz" "${host}:${RELEASES}/"; then  # +4 if (nesting 3)
  32     │           break
  33 +1  │         elif [[ "${attempt}" -eq 3 ]]; then  # +1 elif
  34     │           echo "upload to ${host} failed" >&2
  35     │           return 1
  36 +1  │         else  # +1 else
  37     │           sleep $((attempt * 2))
  38     │         fi
  39     │       done
  40     │       deploy::switch "${host}" "${version}"
  41     │     fi
  42     │   done
  43     │ }
```

The costs grow with depth. The cheapest fixes are the ones that remove a
level: an early `continue` instead of an `else`, or a loop moved into a
function of its own.

### Compared with the specification

The specification has no bash section, and SonarQube computes no cognitive
complexity for shell. bashq follows the specification where bash has an
equivalent construct: `case` like `switch`, counted once; `elif` and `else`
as hybrid increments; sequences of logical operators; recursion; jumps to a
label for `break N`. It adds the two rules bash needs: a test followed by
`&&`/`||` is an `if`, and `|| return` after a command is free error
handling.

## Cyclomatic complexity

McCabe's cyclomatic complexity is the number of independent paths through a
function: 1, plus 1 for each decision.

| Decision | +1 for each |
| --- | --- |
| `if`, `elif` | the keyword |
| `for`, `while`, `until`, `select` | the loop |
| `case` | each branch except the catch-all `*)` |
| `&&`, `\|\|` | operator, in lists, `[[ ]]` and `(( ))`, error handling included |
| `? :` | ternary operator |

It answers a different question from cognitive complexity: "how many test
cases does this need?" rather than "how hard is this to read?". Flat code
with many branches has a high cyclomatic but a low cognitive complexity.
The [`cyclomatic-complexity`](rules.md#cyclomatic-complexity) rule is off by
default.

## Length

The length of a function is its number of lines, from the line of its name
to the line of its closing brace, blank and comment lines included. The
length of a file is its number of lines.

## Line width

The width of a line is its number of characters: Unicode characters, not
bytes, a tab counts 1, and the `\r` of a CRLF line ending does not count.
It is the one measure read on the text rather than on the syntax tree:
comments, strings and heredoc bodies count, since they are as wide to read
as code.

## Nesting depth

The nesting depth of a function is how many control structures (`if`,
`case`, loops, and lists that start with a test) are open at its deepest
point. `bashq check` reports the line of the structure that reaches it.

```bash
f() {
  for x in "$@"; do         # depth 1
    if [[ -f $x ]]; then    # depth 2
      while read -r l; do   # depth 3
        :
      done <"$x"
    fi
  done
}
```

## Lines of code and comments

| Measure | Counts |
| --- | --- |
| lines | every line |
| code lines | lines with something other than blanks and a comment (heredoc bodies count) |
| comment lines | lines where a comment starts, including a shebang and a comment after code |

A line like `echo a # note` is both a code line and a comment line.

## Duplicated lines

bashq turns every script into a sequence of units:

- one unit per simple command, printed back in shfmt's canonical form, so
  indentation, spacing and line breaks do not matter. The comments are
  dropped;
- one unit per keyword of a compound command: `if`, `then`, `elif`,
  `else`, `fi`, a loop header, `do`, `done`, `case … in`, each pattern,
  `esac`, and a function header. The header does not include the function's
  name, so a function copied under another name is still a copy.

A run of at least **15 equal units** spanning at least **10 lines** in each
copy is a duplicated block. Both thresholds are set in
[`duplication`](configuration.md#duplication). A block can repeat within one
file or across files. Two copies of a block that overlap (a run of identical
lines) are not a duplicate of each other.

The duplicated lines of a file are the lines that belong to any copy. The
density in `bashq metrics` is duplicated lines over total lines.

Copies with renamed variables (type 2 clones) are not detected: `"${host}"`
and `"${server}"` are different units.
