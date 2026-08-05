#!/usr/bin/env bash
# show — the four display commands, in about fifteen seconds.
#
#   tinyshow
#
# No questions, no pauses: this is the half of tiny that draws rather than
# asks, so there is nothing to answer. Everything you see is on stderr.

set -uo pipefail
command -v tiny >/dev/null || { echo "tiny is not on PATH"; exit 1; }

b=$'\e[1m'; d=$'\e[2m'; o=$'\e[0m'
step() { printf '\n%s▐ %s%s  %s%s%s\n\n' "$b" "$1" "$o" "$d" "${2:-}" "$o"; }

# ── say ──────────────────────────────────────────────────────────────────────
step "tiny say" "una riga, nello stile del tuo tema"
tiny say "Deploy completato"
tiny say "Niente da fare" --style hint
tiny say "Build fallita" --style error
tiny say "Piano di esecuzione" --style header
printf 'letto da una pipe\ntre righe\ntutte stilate\n' | tiny say --style note

# ── box ──────────────────────────────────────────────────────────────────────
step "tiny box" "riquadro, largo in celle — non in byte"
printf 'Verranno riscritti 3 file.\nNessuna migrazione in sospeso.\n' | tiny box "Piano" --width 52
printf 'larghezza corretta anche in giapponese: 日本語のテキスト\ne con accenti decomposti: cafe\xcc\x80\n' | tiny box "Celle" --width 52

# ── bar ──────────────────────────────────────────────────────────────────────
step "tiny bar" "una riga sola, riscritta sul posto"
for i in $(seq 0 20); do
  tiny bar "$i" 20 "file elaborati"
  sleep 0.06
done

# ── spin ─────────────────────────────────────────────────────────────────────
step "tiny spin" "spinner mentre gira un comando vero"
tiny spin "Compilo il progetto" -- sleep 1.2

tiny spin "Questo fallisce" -- sh -c 'echo "errore del figlio" >&2; exit 3'
printf '%s  ↑ codice di uscita propagato: %s%s\n' "$d" "$?" "$o"

step "e il payload resta pulito" "lo spinner è su stderr, stdout passa intatto"
printf '%s  $ tiny spin "Genero" -- echo %s | wc -c%s\n' "$b" "'{\"ok\":true}'" "$o"
tiny spin "Genero" -- echo '{"ok":true}' | wc -c

printf '\n%s  tinytour per il resto · tiny -h per tutto%s\n\n' "$d" "$o"
