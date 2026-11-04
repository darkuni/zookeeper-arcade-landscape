# Zoo Keeper+ - widescreen native port

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

Arrows move, Z / Space jump, 5 coin, 1 / 2 start, L cycle view (widescreen / original / cabinet 4:3), P Classic / Plus panels, F11 full screen, Esc quit.
Service switches: F1 Test Advance, F2 Next Line, F3 Slew Up, F4 Slew Down.

**Gamepad:** D-pad / left stick move, A or B jump, Select = coin, Start = 1 player, X or Y = 2 players,
right bumper = change view, left bumper = Plus panels on / off, hold Select + Start for a second to quit.

## Classic or Plus

The game itself is never changed - Plus only adds read-outs in the widescreen side panels. For the plain original:
press **P** (or the left bumper) to switch the panels off, press **L** for the original square picture, or start the
program with `-classic` (original square view, no panels). `-plus` goes back to widescreen with panels. The choice is remembered.

Plus shows:

- **Round** - read from the game's own round counter. **Bonus screen** - the original plays its bonus screen after every second round (it tests bit 1 of its level counter), so this counts down to the next even round.
- **Zoo (right side)** - all six animals are shown from the start of a game, each with two counters: **HELD** (animals still inside the zoo when you complete a round, the ones the game pays a bonus for) and **NET** (animals you caught and sent back to the cage). Bright numbers are this game, dim numbers are all-time. The pictures are copied from the game's own screen by a quick background run on first launch (a few seconds, then saved), so no artwork is bundled; a `?` shows until a picture is ready.
- **Jumps** and **Steps** - counted from your controls while a real game is running (a step is 8 frames of walking). They are estimates, not values from the game.
- **All time** - games, best round, jumps, steps, saved in `stats.json` next to the settings.

## Widescreen

On the play screen the field is centred on a 16:9 canvas and the score, keepers and player 2 boxes move to the
side panels. Title and service screens show the whole picture.

## First run

The game needs its settings memory (NVRAM) initialised. The port does that automatically on first launch
(about a second or two before the window opens) and stores it in your user config folder under `zookeeper/`, together with `settings.json` and `stats.json`.

## Status

Boots, attract mode, coin / start, gameplay, service menus, sound all work in headless testing. Window, audio
and gamepad behaviour on real hardware are still being tested.

## Releases

Publishing a GitHub release runs `.github/workflows/release.yml`, which builds `zookeeper.exe` (Windows),
`ZooKeeper-linux.tar.gz` (Linux x86_64, Steam Deck / SteamOS desktop mode) and `ZooKeeper-mac.zip` (universal macOS app).
- **High score and Best jump (top right)** - the high score is the game's own best score (first entry of its high-score table). Best jump is the most points one landing has paid (the game pays 100, 500, 2,000, 6,000 ... up to 2,000,000 depending on how many animals Zeke cleared); it is read from the points popup the game draws and kept across games, with this game's best in dim text under it.
- **Two players** - both score boxes are always shown, each with a PLAYER 1 / PLAYER 2 label (the player whose turn it is is highlighted, so you can watch the other player's score while they wait). Every stat is kept separately per player: round, jumps, steps, animals held, nets, best jump and the all-time totals. The side panels switch to the player whose turn it is and say whose stats they are. Starting a 2 player game is detected automatically.
