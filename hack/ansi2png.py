#!/usr/bin/env python3
"""Render an ANSI (SGR truecolor) frame to a PNG so a screenshot can be looked at.

    ./aitop --screenshot 160x45 | hack/ansi2png.py out.png

Dev tool only. Understands SGR 0/1/22/39/49 and 38;2/48;2 truecolor, which is
everything lipgloss emits for a truecolor profile.
"""
import re
import sys

from PIL import Image, ImageDraw, ImageFont

FONT_REG = "/usr/share/fonts/TTF/JetBrainsMonoNerdFontMono-Regular.ttf"
FONT_BOLD = "/usr/share/fonts/TTF/JetBrainsMonoNerdFontMono-Bold.ttf"
BG = (0x0B, 0x0F, 0x17)  # kitty background on this box
FG = (0xB5, 0xC1, 0xD4)
SIZE = 18

SGR = re.compile(r"\x1b\[([0-9;]*)m")


def parse(text):
    """Yield (line_index, [(char, fg, bg, bold), ...])."""
    rows = []
    for line in text.split("\n"):
        cells = []
        fg, bg, bold = FG, None, False
        pos = 0
        for m in SGR.finditer(line):
            for ch in line[pos:m.start()]:
                cells.append((ch, fg, bg, bold))
            pos = m.end()
            params = [int(p) if p else 0 for p in m.group(1).split(";")] if m.group(1) else [0]
            i = 0
            while i < len(params):
                p = params[i]
                if p == 0:
                    fg, bg, bold = FG, None, False
                elif p == 1:
                    bold = True
                elif p == 22:
                    bold = False
                elif p == 39:
                    fg = FG
                elif p == 49:
                    bg = None
                elif p in (38, 48) and i + 4 < len(params) and params[i + 1] == 2:
                    col = (params[i + 2], params[i + 3], params[i + 4])
                    if p == 38:
                        fg = col
                    else:
                        bg = col
                    i += 4
                i += 1
        for ch in line[pos:]:
            cells.append((ch, fg, bg, bold))
        rows.append(cells)
    while rows and not rows[-1]:
        rows.pop()
    return rows


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else "frame.png"
    rows = parse(sys.stdin.read())
    reg = ImageFont.truetype(FONT_REG, SIZE)
    bold = ImageFont.truetype(FONT_BOLD, SIZE)
    cw = reg.getlength("M")
    ch = int(SIZE * 1.3)
    width = int(max(len(r) for r in rows) * cw) + 2 * SIZE
    height = ch * len(rows) + 2 * SIZE
    img = Image.new("RGB", (width, height), BG)
    d = ImageDraw.Draw(img)
    for y, cells in enumerate(rows):
        for x, (c, fg, bg, b) in enumerate(cells):
            px, py = SIZE + x * cw, SIZE + y * ch
            if bg:
                d.rectangle([px, py, px + cw, py + ch], fill=bg)
            if c != " ":
                d.text((px, py), c, font=bold if b else reg, fill=fg)
    img.save(out)
    print(out, img.size)


if __name__ == "__main__":
    main()
