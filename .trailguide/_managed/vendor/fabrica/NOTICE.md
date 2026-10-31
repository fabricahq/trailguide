# Source notices

Some React, TypeScript, code-design, and testing rules adapt the [TypeScript Style Guide](https://github.com/mkosir/typescript-style-guide/tree/86bebd58a987e23277dba02028c0ee2d6ffb5073) by mkosir at commit 86bebd58a987e23277dba02028c0ee2d6ffb5073. Its notice follows unchanged:

## TypeScript Style Guide

MIT License

Copyright (c) 2022 mkosir

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Vercel Agent Skills

React, JavaScript, and Performance rules include adaptations of Vercel Labs Agent Skills at commit `4ec6f84b61cd3c931046c3e6e398f3ae7de372f7`, including its React best practices and composition patterns. Each adapted rule identifies its original file.

The [upstream README at that revision](https://github.com/vercel-labs/agent-skills/blob/4ec6f84b61cd3c931046c3e6e398f3ae7de372f7/README.md#license) declares MIT. That revision contains no separate license file, and its README supplies no copyright notice. We credit Vercel Labs and the upstream contributors; we do not claim ownership of their work. The MIT permission and warranty terms are reproduced below.

MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## React documentation references

The `official-*` React rules express documented React behavior in their own wording and examples, with links to the relevant official pages. The React documentation itself is licensed under [CC BY 4.0](https://github.com/reactjs/react.dev/blob/main/LICENSE-DOCS.md); this library's MIT declaration does not relicense that documentation. Other React rules retain their cited documentation references.

## TanStack Agent Skills

TanStack Query and TanStack Router rules include adaptations of Deckard Gerritsen's TanStack Agent Skills at commit `0e8bcdc6af4959739e0f6a2dfb35dc70d513940a`. Each adapted rule identifies its original file.

The [README at the source revision](https://github.com/DeckardGer/tanstack-agent-skills/blob/0e8bcdc6af4959739e0f6a2dfb35dc70d513940a/README.md#license) declares MIT. That revision has no separate license file. The following notice is reproduced unchanged from the [upstream LICENSE at `b2fc14c`](https://github.com/DeckardGer/tanstack-agent-skills/blob/b2fc14c974b2ca09b38de09c28237f240175205b/LICENSE); the rule text remains attributed to its original revision.

MIT License

Copyright (c) 2026 Deckard Gerritsen

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Zustand references

The Zustand rules are original guidance informed by documentation and examples, not copies of an upstream rule set. They retain references to the relevant APIs and draw on:

- [ExplainX's rendering of Zustand guidance from jezweb/claude-skills](https://explainx.ai/skills/jezweb/claude-skills/zustand-state-management).
- [Zustand guidance in awesome-cursor-rules-mdc](https://github.com/sanjeed5/awesome-cursor-rules-mdc/blob/main/rules-mdc/zustand.mdc).

These references do not transfer ownership of the upstream material to Fabrica.

## pstack

The library curates skills from [pstack](https://github.com/cursor/plugins/tree/main/pstack) by Lauren Tan, at commit `699e1e7488c53c05ca13409f40da83cd37682091` of `cursor/plugins`. Projects fetch those skills from that repository, so the skills are not part of this library and its MIT declaration does not cover them. pstack's own MIT license covers them; it keeps the notice in [`pstack/LICENSE`](https://github.com/cursor/plugins/blob/699e1e7488c53c05ca13409f40da83cd37682091/pstack/LICENSE) rather than in each skill folder, so it is reproduced unchanged below.

Four of the library's own skills draw on pstack, and the notice below also applies to the material they copy. `show-me-your-work` adapts pstack's skill of that name at commit `699e1e7488c53c05ca13409f40da83cd37682091`, changing the steps that read Cursor's transcripts and choose the review model, adding an outcome for harnesses that can't start a reviewer, and dropping its mentions of pstack skills the library doesn't install. `build` adapts steps from the Feature and Autonomous run playbooks of pstack's `poteto-mode` skill. `create-verification-skill`, including its feature-map examples, and `maintain-verification-skill` adapt the corresponding skills at the same pinned commit to use project-owned skill locations instead of Cursor-only paths; maintenance also bounds source reviews by the host's concurrency limit and supports sequential review. The bundled search example restores its query before capturing evidence.

MIT License

Copyright (c) 2026 Lauren Tan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Testing philosophy reference

`assets/testing-philosophy.md` is based on Yevgeniy Brikman's talk [Agility Requires Safety](https://www.ybrikman.com/blog/2016/02/14/agility-requires-safety/) (2016). The asset paraphrases and extends its ideas in original wording; no text or slides from the talk are reproduced, and the talk remains his work.

## dzhng/skills

The `ratchet-files` skill adapts [dzhng/skills](https://github.com/dzhng/skills/tree/dc8f546c3adaa615ab86e8328449ae6efe23aca4/skills/engineering/ratchet-files) by David Zhang at commit dc8f546c3adaa615ab86e8328449ae6efe23aca4: the skill's frontmatter is quoted for YAML and a link to a sibling skill is made absolute, and the script is unchanged. Its notice follows unchanged:

MIT License

Copyright (c) 2026 David Zhang

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
