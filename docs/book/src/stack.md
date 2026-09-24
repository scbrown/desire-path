# The stack

Caboodle installs these together and proves each one works; every tool also stands alone.

| tool | what it gives your agents |
|---|---|
| [caboodle](https://github.com/scbrown/caboodle) | one wizard that installs the stack and proves it works |
| [quipu](https://github.com/scbrown/quipu) | a knowledge graph that refuses facts that break its rules |
| [camayoc](https://github.com/scbrown/camayoc) | the starter vocabulary, and how new knowledge earns its way in |
| [bobbin](https://github.com/scbrown/bobbin) | search and context over your repositories, served over MCP |
| [yupana](https://github.com/scbrown/yupana) | which code calls which: the blast radius before an edit |
| [desire-path](https://github.com/scbrown/desire-path) **(you are here)** | the tool calls your agents get wrong, so you can fix them |

Desire Path supplies evidence about failed tool calls. Bobbin supplies repository
search, and Yupana supplies structural queries. Quipu and Camayoc provide governed
knowledge; Caboodle installs and checks the tools. Basic Desire Path capture and
queries work locally without any of the other services.

The optional signposting mode can suggest a search or structural command after a
weak search. Read its [contract](https://github.com/scbrown/desire-path/blob/main/docs/plans/009-signposting-eval.md) before
turning it on; presence of a tool is not evidence of improved results.
