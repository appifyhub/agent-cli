#!/usr/bin/env sh
# fails for any tracked (or untracked and not ignored) non-empty text file without a final newline

missing=$(git ls-files --cached --others --exclude-standard | while IFS= read -r file; do
    [ -f "$file" ] && [ -s "$file" ] || continue
    grep -Iq . "$file" || continue
    [ -z "$(tail -c 1 "$file")" ] || printf "%s\n" "$file"
done)

if [ -n "$missing" ]; then
    printf "%s\n" "$missing" | sed 's/^/missing final newline: /' >&2
    exit 1
fi
