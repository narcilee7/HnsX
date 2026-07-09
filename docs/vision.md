# The vision of hns-x(Harness X)

## 我看到了什么

Agent时代，大家越来越多的自己实现Agent，集成到自己的业务场景，这些东西叫做Coze，I8n，现在自己实现的各种Agent实际上落地困难，效果很差劲，而Coding Agent比如Claude Code，Codex，他们的能力绝对是SOTA，那我看到了一个问题：为什么不直接用这些Coding Agent，很多时候他们的效果反而很好。

那么看到这个问题，整体来看需要一套Harness，将长期以来用户积累的优秀的领域知识，领域Skill，Rules整合起来，Haress最强大的Agent。

现在有claude code，codex，openClaw，之后还会有更多，我们要做的不是做一个更好的agent底座，而是为用户提供这套harness约束的能力，将这些Agent整合起来，提供一个更好的Agent服务。

Harness as a Service

### 我们要做什么

- Harness建设：支持用户改造SystemPrompt，构建业务知识库、上传Skills，Rules等定义自己的Harness；
- Agent运行时的编排：自由编排Agent&Harness，不只是一个Workflow，一个Yaml/Json/Toml编排好，而是细粒度的编排，到sessions等粒度；
- 评估：Agent&Harness能力强不强，需要评估，好的评测集才能让Agent进化；
- 监控&观测：Agent是否可以进化，是Harness能力是否进化，我们要细粒度到agent&model层，把agent每一个token、function call做的事情都要弄清楚；
- Sandbox：Sandbox决定了强度和能做的范围，对人来说，什么能做什么不能做用户要感知要可控，而不是一个玩具；
- 部署：我们要支持部署，让用户可以方便地部署和管理Harness和Agent；

### 产品形态

- SDK：用户可以自己定义一个Agent，然后将其部署到Harness上运行
- 平台：用户可以在平台上管理Harness和Agent，包括部署、监控、评估等
