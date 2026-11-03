# Zoo Keeper - widescreen native port

Taito's 1982 Zoo Keeper (Qix-family hardware) as a native Go + Ebitengine program, re-laid-out for 16:9.

## How it works (honest version)

This is **not** a static recompilation and not a routine-by-routine rewrite. It is a native Go port of the
whole machine: cycle-stepped 6809 (data + video), 6802 (sound) and 68705 (coin MCU) cores, 6821 PIAs, a 6845
CRTC and the DAC sound path, all written in Go. The original ROMs run unmodified on top of it. The ROMs are
never included and are loaded at runtime.

## ROMs (required, not included)

Your own copy of the MAME `zookeep` set, as `zookeep.zip`, in the same folder as the game (or in a `roms` folder,
the working directory, your home folder, or pass a zip / folder path as an argument). It needs:
`za3`-`za10`, `za12`-`za19`, `za25`-`za27` and `za_coin.bin`. If anything is missing a window lists what.

## Controls

Arrows move, Z / Space jump, 5 coin, 1 / 2 start, L cycle view (widescreen / original / cabinet 4:3), F11 full screen, Esc quit.
Service switches: F1 Test Advance, F2 Next Line, F3 Slew Up, F4 Slew Down.

**Gamepad:** D-pad / left stick move, A or B jump, Select = coin, Start = 1 player, X or Y = 2 players,
right bumper = change view, hold Select + Start for a second to quit.

## Widescreen

On the play screen the field is centred on a 16:9 canvas and the score, keepers and player 2 boxes move to the
side panels. Title and service screens show the whole picture.

## First run

The game needs its settings memory (NVRAM) initialised. The port does that automatically on first launch
(about a second or two before the window opens) and stores it in your user config folder under `zookeeper/`.

## Status

Boots, attract mode, coin / start, gameplay, service menus, sound all work in headless testing. Window, audio
and gamepad behaviour on real hardware are still being tested.

## Releases

Publishing a GitHub release runs `.github/workflows/release.yml`, which builds `zookeeper.exe` (Windows),
`ZooKeeper-linux.tar.gz` (Linux x86_64, Steam Deck / SteamOS desktop mode) and `ZooKeeper-mac.zip` (universal macOS app).
