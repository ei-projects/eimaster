# Evil Islands Master Server

This project is an implementation of a master server for Evil Islands: Curse of the Lost Soul game.
It is at a very early stage of development, but nevertheless the main functionality is implemented.

## How to build

1. You need the golang 1.15 or higher and `make` tool
2. If so, simply run `make`

## Examples

1. Running server: `eimaster server run --addr :28004 --http-addr 8000`
2. Getting servers list: `eimaster client get a3master.nival.com:28004`

## How to configure the game to use master server

1. Run regedit.exe
2. Open the key `HKEY_CURRENT_USER\Software\Nival Interactive\EvilIslands\Network Settings`
3. Specify the address of your server with port in the `Master Server Name` value
   (e.g. `example.com:28010`)
