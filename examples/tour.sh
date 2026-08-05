#!/usr/bin/env bash
# tour — see everything tiny does, in one sitting.
#
#   tinytour            the whole tour
#   tinytour paths      one section only (see the list below)
#
# Self-contained on purpose: every wizard here is either the built-in demo or a
# heredoc piped into `tiny -`, so the tour runs from any directory on a machine
# that has nothing checked out. That is also the point being demonstrated.

set -uo pipefail

command -v tiny >/dev/null || { echo "tiny is not on PATH"; exit 1; }

bold=$'\e[1m'; dim=$'\e[2m'; cyan=$'\e[38;5;51m'; off=$'\e[0m'

section() {
  # Padded by characters, not bytes: a title with an accent in it would push
  # the box out of shape under %-57s.
  local width=57 chars pad
  chars=$(printf '%s' "$1" | wc -m | tr -d ' ')
  pad=$(( width - chars )); [ "$pad" -lt 0 ] && pad=0
  printf '\n%s┌───────────────────────────────────────────────────────────┐%s\n' "$cyan" "$off"
  printf '%s│%s %s%s%s%*s %s│%s\n' "$cyan" "$off" "$bold" "$1" "$off" "$pad" "" "$cyan" "$off"
  printf '%s└───────────────────────────────────────────────────────────┘%s\n' "$cyan" "$off"
  [ -n "${2:-}" ] && printf '%s%s%s\n' "$dim" "$2" "$off"
  return 0
}

pause() {
  # A tour with nobody watching should still run end to end, so a missing
  # terminal skips the wait rather than blocking forever.
  [ -t 0 ] || return 0
  printf '\n%s  ⏎ per continuare%s' "$dim" "$off"
  read -r _
}

run() {
  printf '\n%s  $ %s%s\n\n' "$bold" "$*" "$off"
  "$@"
}

# ── 1. what it is ────────────────────────────────────────────────────────────
tour_hello() {
  section "1 · Cos'è" "Un binario solo, due dipendenze, YAML dentro e JSON fuori."
  run tiny -version
  printf '\n  binario: %s byte\n' "$(wc -c < "$(command -v tiny)" | tr -d ' ')"
  printf '  temi:    %s\n' "$(tiny -list-themes | tr '\n' ' ')"
  pause
}

# ── 2. the whole wizard as arguments ─────────────────────────────────────────
tour_argv() {
  section "2 · Un wizard scritto come argomenti" \
    "Nessun file. Frecce per muoverti, invio per rispondere."
  run tiny select framework SwiftUI UIKit AppKit + confirm git yes
  pause
}

# ── 3. every look, same answers ──────────────────────────────────────────────
tour_themes() {
  section "3 · Ogni tema, la stessa domanda" \
    "L'estetica cambia; la risposta no. È un test, non una promessa."
  local spec='steps:
  - type: select
    key: colour
    title: "Scegli un colore"
    options: [rosso, verde, blu]
    default: verde'
  local t sums=()
  for t in $(tiny -list-themes); do
    printf '\n%s  ── tema: %s ──%s\n' "$bold" "$t" "$off"
    if [ -t 1 ]; then
      sums+=("$(printf '%s' "$spec" | tiny -theme "$t" -compact - | tee /dev/tty | tail -1)")
    else
      sums+=("$(printf '%s' "$spec" | tiny -theme "$t" -compact -)")
    fi
  done
  printf '\n%s  payload sotto tutti i temi:%s\n' "$bold" "$off"
  printf '  %s\n' "${sums[@]}" | sort -u
  printf '%s  una riga sola = i temi non toccano le risposte%s\n' "$dim" "$off"
  pause
}

# ── 4. filtering ─────────────────────────────────────────────────────────────
tour_fuzzy() {
  section "4 · Ricerca fuzzy su una lista lunga" \
    "Scrivi 'aps' o 'ue1': la lista si riordina, il migliore va sotto il cursore."
  tiny - <<'YAML'
steps:
  - type: search
    key: region
    title: "Regione"
    match: fuzzy
    height: 8
    default: eu-west-1
    options:
      - eu-west-1
      - eu-west-2
      - eu-central-1
      - eu-north-1
      - us-east-1
      - us-west-2
      - ca-central-1
      - sa-east-1
      - af-south-1
      - me-central-1
      - ap-south-1
      - ap-east-1
      - ap-northeast-1
      - ap-southeast-1
      - ap-southeast-2
YAML
  pause
}

# ── 5. labels and descriptions ───────────────────────────────────────────────
tour_labels() {
  section "5 · Etichette e descrizioni" \
    "Vedi 'Production', il chiamante riceve 'prod'. L'etichetta è digitabile."
  run tiny - <<'YAML'
steps:
  - type: select
    key: env
    title: "Ambiente"
    default: staging
    options:
      - value: dev
        label: Development
        description: nessuno viene svegliato di notte
      - value: staging
        label: Staging
        description: dati come in produzione, traffico no
      - value: prod
        label: Production
        description: utenti veri, reperibilità vera
YAML
  pause
}

# ── 6. multiselect with bounds ───────────────────────────────────────────────
tour_multi() {
  section "6 · Multiselect con limiti" \
    "Spazio per marcare, ctrl-a per tutti. Min 1, max 3: provaci."
  run tiny - <<'YAML'
steps:
  - type: multiselect
    key: components
    title: "Cosa spedisci"
    options: [api, worker, scheduler, web, migrations, docs]
    default: "api, worker"
    min_select: 1
    max_select: 3
YAML
  pause
}

# ── 7. validation ────────────────────────────────────────────────────────────
tour_validation() {
  section "7 · Validazione dichiarativa" \
    "Prova a scrivere 'X': la regola sta nello spec, non nel tuo codice."
  run tiny - <<'YAML'
steps:
  - type: input
    key: service
    title: "Nome del servizio"
    default: aurora-api
    pattern: "^[a-z][a-z0-9-]{2,23}$"
    message: "Minuscole, cifre e trattini; 3-24 caratteri."
    required: true
YAML
  pause
}

# ── 8. the full showcase ─────────────────────────────────────────────────────
tour_demo() {
  section "8 · Lo showcase: 20 step, ogni tipo" \
    "Ctrl-C in qualsiasi momento; il terminale viene rimesso a posto."
  run tiny demo
  pause
}

# ── 9. the same file, headless ───────────────────────────────────────────────
tour_headless() {
  section "9 · Lo stesso file, senza nessuno che risponde" \
    "Un file, due modi. È la ragione per cui tiny esiste."
  run tiny -defaults -compact demo
  pause
}

# ── 10. paths ────────────────────────────────────────────────────────────────
tour_paths() {
  section "10 · Ogni forma che il wizard può prendere" \
    "Non chiede niente, non legge niente: si usa su uno spec che ti hanno mandato."
  printf '\n%s  $ tiny -paths demo | head -6%s\n\n' "$bold" "$off"
  tiny -paths demo | head -6
  printf '\n  forme distinte in tutto: %s\n' "$(tiny -paths demo | wc -l | tr -d ' ')"
  pause
}

# ── 11. meta ─────────────────────────────────────────────────────────────────
tour_meta() {
  section "11 · Da dove viene questo payload" \
    "Chi ha risposto, con quale spec, e quali domande non sono mai state fatte."
  printf '\n%s  $ tiny -defaults -meta demo%s\n\n' "$bold" "$off"
  if command -v jq >/dev/null; then
    tiny -defaults -meta demo | jq '._meta'
  else
    tiny -defaults -meta demo | head -14
  fi
  pause
}

# ── 12. the payload is only the payload ──────────────────────────────────────
tour_streams() {
  section "12 · Su stdout c'è solo il payload" \
    "Le domande vanno su stderr, quindi la pipe funziona mentre qualcuno risponde."
  printf '\n%s  $ tiny select colour rosso verde blu | jq -r .colour%s\n\n' "$bold" "$off"
  if command -v jq >/dev/null; then
    tiny select colour rosso verde blu | jq -r '.colour'
  else
    tiny select colour rosso verde blu
  fi
  pause
}

# ── 13. what a spec cannot do ────────────────────────────────────────────────
tour_inert() {
  section "13 · Cosa uno spec NON può fare" \
    "Niente comandi, niente rete, nessun file che non gli hai indicato."
  printf '\n%s  uno spec che prova a scegliersi l'"'"'editor:%s\n\n' "$bold" "$off"
  printf 'steps:\n  - {type: editor, key: n, title: N, editor: "/usr/bin/env sh"}\n' | sed 's/^/    /'
  printf '\n%s  → il campo viene letto e ignorato: l'"'"'editor arriva solo da $VISUAL/$EDITOR.%s\n' "$dim" "$off"
  printf '\n%s  uno spec che prova a includere un file fuori dalla sua cartella:%s\n\n' "$bold" "$off"
  local dir; dir="$(mktemp -d)"
  printf 'include: ["../fuori.yaml"]\nsteps:\n  - {type: input, key: a, title: A}\n' > "$dir/w.yaml"
  printf '    '; tiny "$dir/w.yaml" 2>&1 | head -2
  rm -rf "$dir"
  pause
}

# ── 14. the display commands ─────────────────────────────────────────────────
tour_show() {
  section "14 · L'altra meta: mostrare" \
    "spin, say, box, bar — disegnano invece di chiedere. Niente su stdout."
  tiny say "Deploy completato" --style header
  printf 'Verranno riscritti 3 file.\n' | tiny box "Piano" --width 52
  local i
  for i in $(seq 0 10); do tiny bar "$i" 10 "file elaborati"; sleep 0.05; done
  tiny spin "Compilo" -- sleep 1
  tiny spin "Fallisce" -- sh -c 'echo "errore" >&2; exit 3'
  printf '%s  codice di uscita del figlio: %s%s\n' "$dim" "$?" "$off"
  printf '\n%s  $ tiny spin "Genero" -- echo %s | wc -c%s\n' "$bold" "'{\"ok\":true}'" "$off"
  tiny spin "Genero" -- echo '{"ok":true}' | wc -c
  pause
}

tour_end() {
  section "Fine" "tiny -h per tutti i flag · https://github.com/martino-vigiani/tiny"
}

sections=(hello argv themes fuzzy labels multi validation demo headless paths meta streams inert show)

if [ $# -gt 0 ]; then
  for name in "$@"; do
    if ! declare -F "tour_$name" >/dev/null; then
      echo "sezione sconosciuta: $name"
      echo "disponibili: ${sections[*]}"
      exit 2
    fi
    "tour_$name"
  done
  exit 0
fi

for name in "${sections[@]}"; do "tour_$name"; done
tour_end
