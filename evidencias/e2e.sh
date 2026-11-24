#!/usr/bin/env bash
# Roteiro de teste ponta a ponta (E2E) da API AgroStock.
# Uso: bash evidencias/e2e.sh [URL_BASE]   (padrão http://localhost:8080)
# Requer a API rodando com banco SQL Server e RESET_TOKEN_NA_RESPOSTA=true.
B=${1:-http://localhost:8080}
J=$(mktemp); C=$(mktemp)
S=$(date +%s)
LOGIN="e2e_$S"; SENHA="SenhaTeste123"; EMAIL="e2e_$S@teste.local"
OK=0; FALHA=0

req() { # metodo caminho [corpo] -> imprime status; corpo da resposta em $J
  local m=$1 p=$2 d=$3
  if [ -n "$d" ]; then
    curl -s -o "$J" -w '%{http_code}' -b "$C" -c "$C" -X "$m" -H 'Content-Type: application/json' -H 'Origin: http://localhost:5173' --data "$d" "$B$p"
  else
    curl -s -o "$J" -w '%{http_code}' -b "$C" -c "$C" -X "$m" -H 'Origin: http://localhost:5173' "$B$p"
  fi
}
caso() { # id descricao esperado obtido
  if [ "$3" = "$4" ]; then r=PASSOU; OK=$((OK+1)); else r=FALHOU; FALHA=$((FALHA+1)); fi
  printf '%-6s | %-62s | esperado %-4s | obtido %-4s | %s\n' "$1" "$2" "$3" "$4" "$r"
  printf '         resposta: %s\n' "$(head -c 300 "$J" | tr -d '\n')"
}
campo() { grep -o "\"$1\":[^,}]*" "$J" | head -1 | cut -d: -f2- | tr -d '" '; }

echo "# E2E AgroStock API | $(date '+%Y-%m-%d %H:%M:%S %z') | base $B | commit $(git rev-parse --short HEAD 2>/dev/null)"
caso E01 "Health check"                                   200 "$(req GET /health)"
caso E02 "Acesso a /saldos sem sessao e negado"           401 "$(req GET /saldos)"
caso E03 "Cadastro de usuario valido"                     201 "$(req POST /usuarios "{\"login\":\"$LOGIN\",\"email\":\"$EMAIL\",\"senha\":\"$SENHA\"}")"
caso E04 "Cadastro com login duplicado e rejeitado"       409 "$(req POST /usuarios "{\"login\":\"$LOGIN\",\"email\":\"x$EMAIL\",\"senha\":\"$SENHA\"}")"
caso E05 "Login com senha errada"                         401 "$(req POST /sessao "{\"login\":\"$LOGIN\",\"senha\":\"errada12345\"}")"
caso E06 "Login valido cria sessao (cookie)"              200 "$(req POST /sessao "{\"login\":\"$LOGIN\",\"senha\":\"$SENHA\"}")"
caso E07 "Consulta da sessao atual"                       200 "$(req GET /sessao)"
caso E08 "Cadastro de insumo valido"                      201 "$(req POST /insumos '{"nome":"E2E-Semente Milho","categoria":"Sementes","unidade":"kg","minimo":50}')"
INS=$(campo id)
caso E09 "Cadastro de insumo sem nome e rejeitado"        422 "$(req POST /insumos '{"nome":"","categoria":"Sementes","unidade":"kg","minimo":1}')"
caso E10 "Listagem de insumos"                            200 "$(req GET /insumos)"
VAL=$(date -d '+10 days' +%F 2>/dev/null || date -v+10d +%F)
caso E11 "Cadastro de lote (validade em 10 dias)"         201 "$(req POST /lotes "{\"insumoId\":$INS,\"codigo\":\"E2E-L$S\",\"validade\":\"$VAL\"}")"
LOTE=$(campo id)
caso E12 "Lote com codigo duplicado no insumo"            409 "$(req POST /lotes "{\"insumoId\":$INS,\"codigo\":\"E2E-L$S\",\"validade\":\"$VAL\"}")"
caso E13 "Lote com data invalida e rejeitado"             422 "$(req POST /lotes "{\"insumoId\":$INS,\"codigo\":\"E2E-X$S\",\"validade\":\"31/12/2026\"}")"
caso E14 "Entrada de 100 kg no lote"                      201 "$(req POST /movimentacoes "{\"loteId\":$LOTE,\"tipo\":\"E\",\"quantidade\":100,\"motivo\":\"compra\",\"chaveRequisicao\":\"e2e-$S-1\"}")"
caso E15 "Reenvio idempotente (mesma chave e corpo)"      200 "$(req POST /movimentacoes "{\"loteId\":$LOTE,\"tipo\":\"E\",\"quantidade\":100,\"motivo\":\"compra\",\"chaveRequisicao\":\"e2e-$S-1\"}")"
caso E16 "Mesma chave com dados diferentes e rejeitada"   409 "$(req POST /movimentacoes "{\"loteId\":$LOTE,\"tipo\":\"E\",\"quantidade\":5,\"chaveRequisicao\":\"e2e-$S-1\"}")"
caso E17 "Saida de 30 kg"                                 201 "$(req POST /movimentacoes "{\"loteId\":$LOTE,\"tipo\":\"S\",\"quantidade\":30,\"chaveRequisicao\":\"e2e-$S-2\"}")"
echo "         saldoLote apos saida: $(campo saldoLote) (esperado 70)"
caso E18 "Saida maior que o saldo e bloqueada"            409 "$(req POST /movimentacoes "{\"loteId\":$LOTE,\"tipo\":\"S\",\"quantidade\":1000,\"chaveRequisicao\":\"e2e-$S-3\"}")"
caso E19 "Quantidade zero e rejeitada"                    422 "$(req POST /movimentacoes "{\"loteId\":$LOTE,\"tipo\":\"E\",\"quantidade\":0,\"chaveRequisicao\":\"e2e-$S-4\"}")"
caso E20 "Historico de movimentacoes"                     200 "$(req GET /movimentacoes)"
caso E21 "Saldos por insumo/lote"                         200 "$(req GET /saldos)"
caso E22 "Alertas de validade"                            200 "$(req GET /alertas)"
grep -q "E2E-L$S" "$J" && echo "         lote E2E-L$S aparece nos alertas (vence em 10 dias)" || echo "         lote E2E-L$S NAO aparece nos alertas"
caso E23 "Esqueci a senha (token em modo dev)"            200 "$(req POST /senha/esqueci "{\"email\":\"$EMAIL\"}")"
TOK=$(campo tokenRedefinicao)
caso E24 "Redefinir senha com token valido"               200 "$(req POST /senha/redefinir "{\"token\":\"$TOK\",\"novaSenha\":\"NovaSenha456\"}")"
caso E25 "Reuso do mesmo token e rejeitado"               400 "$(req POST /senha/redefinir "{\"token\":\"$TOK\",\"novaSenha\":\"OutraSenha789\"}")"
caso E26 "Logout encerra a sessao"                        204 "$(req DELETE /sessao)"
caso E27 "Apos logout, rota protegida nega acesso"        401 "$(req GET /saldos)"
caso E28 "Login com a nova senha"                         200 "$(req POST /sessao "{\"login\":\"$LOGIN\",\"senha\":\"NovaSenha456\"}")"
echo "# Resultado: $OK passaram, $FALHA falharam"
rm -f "$J" "$C"
