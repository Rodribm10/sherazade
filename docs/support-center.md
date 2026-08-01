# Central de suporte

A Central de Suporte transforma uma conversa restrita da lider em um caso
auditavel sem liberar o restante do workspace. O piloto aceita somente o
aplicativo `inaudit`.

## Papeis e superficies

- `reporter`: acessa somente `/support` e os endpoints `/api/support/*` do
  proprio caso;
- `owner` e `admin`: acessam `/support-admin`, a fila interna e as metricas;
- somente `workspace.settings.support.approver_user_id` pode aprovar ou rejeitar
  execucao tecnica; quando ele nao estiver configurado, a decisao fica restrita
  ao `owner` do workspace.

Um caso cria uma issue interna com origem `support_case`. Uma escalada tecnica
cria, de forma idempotente, uma segunda issue com origem `support_technical` e
titulo `INA-*`. Reporter nao recebe acesso a nenhuma dessas issues.

## Configuracao do workspace

O bloco abaixo vive em `workspace.settings`:

```json
{
  "support": {
    "concierge_agent_id": "uuid do agente sem shell e sem repositorio",
    "project_id": "uuid do projeto da fila interna",
    "technical_project_id": "uuid do projeto tecnico do InAudit",
    "approver_user_id": "uuid do aprovador humano",
    "model": "modelo opcional",
    "knowledge_context": "conhecimento aprovado e versionado"
  }
}
```

`concierge_agent_id` e obrigatorio para analise por IA. Os IDs de projeto sao
obrigatorios para materializar as issues internas. IDs invalidos ou de outro
workspace nao concedem acesso e deixam o caso visivel para correcao operacional.

## Anexos

O upload de reporter aceita PNG, JPEG, WEBP ou PDF, ate 10 MB por arquivo e no
maximo cinco anexos por mensagem. O servidor valida a assinatura do arquivo,
nao apenas a extensao. O objeto e vinculado a sessao, workspace e usuario antes
de poder ser lido; falha ao persistir o vinculo remove o objeto enviado.

## Estados tecnicos

1. `em_investigacao_tecnica`: o Support Bridge pode iniciar diagnostico somente
   leitura;
2. `aguardando_aprovacao`: uma spec revisada esta disponivel para decisao;
3. `em_correcao`: a aprovacao autenticada libera apenas a copia isolada;
4. `pronto_para_publicar`: a validacao local passou, mas GitHub, banco e
   producao continuam gates separados;
5. `publicado`: permitido somente depois de deploy pelo fluxo oficial e smoke
   em producao;
6. `aguardando_confirmacao`: a lider confirma `Resolveu` ou reabre o caso.

## Broker de evidencias

As variaveis do backend sao:

```dotenv
MULTICA_SUPPORT_EVIDENCE_URL=https://host/functions/v1/support-evidence-broker
MULTICA_SUPPORT_EVIDENCE_TOKEN=<segredo dedicado>
MULTICA_SUPPORT_EVIDENCE_TIMEOUT=8s
```

O broker e somente leitura, nao segue redirects e nunca entrega credenciais de
banco, token de repositorio ou SQL arbitrario ao Concierge. Consulte
`docs/support-evidence-broker.md` para o contrato completo.

## Validacao minima de release

- migration completa em PostgreSQL descartavel;
- teste positivo de reporter no proprio caso;
- `403` ou `404` para outro reporter, outro workspace e rotas tecnicas;
- anexos validos e rejeicao de conteudo disfarçado;
- criacao unica das issues `SUP-*` e `INA-*`;
- aprovador configurado aceito e outro admin bloqueado;
- resultado tecnico devolvido ao caso;
- build web e testes Go/TypeScript;
- backup, migration, deploy versionado e smoke de producao.
