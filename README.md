# Kodicast

This is a small [DIAL](http://www.dial-multiscreen.org) server that emulates
Chromecast-like devices, and implements the YouTube app. It proxies YouTube
commands from mobile app to Kodi YouTube plugin.

## Fork

This repository is a fork of [https://github.com/sargo/kodicast](https://github.com/sargo/kodicast).
Everything in this README, except this section, was written by [sargo](https://github.com/sargo).

I don't intend to maintain this beyond what works for me, but feel free to
contribute if you'd like.

### Changes from upstream

So far just pinning dependency versions and adding a build to releases.

## Kodi configuration

Turn on JSON-RPC (TCP transport) by enabling "Allow remote control from applications
on other systems" in "Settings > Service > Control" panel.
 
## Installation

I'm going to assume you're running Linux for this installation guide, preferably
Debian Jessie (or newer when their time comes). Debian before Jessie contains
too old versions of certain packages.

First, make sure you have the needed dependencies installed:

 *  golang 1.8+

These can be installed in one go under Debian Jessie (with jessie-backports):

    $ sudo apt-get install golang-1.8

If you haven't already set up a Go workspace, create one now. Some people like
to set it to their home directory, but you can also set it to a separate
directory. In any case, set the environment variable `$GOROOT` to this path:

    $ mkdir golang
    $ cd golang
    $ export GOPATH="`pwd`"

Then get the required packages and compile:

    $ go get -u github.com/sargo/kodicast

To run the server, run the executable `bin/kodicast` relative to your Go
workspace. Any Android phone or iPhone with YouTube app on the same network
should recognize the server and it should be possible to play videos on Kodi.
The Chrome extension doesn't yet work.

    $ bin/kodicast

## Thanks

Big part of Kodicast is taken from
[Plaincast](https://github.com/aykevl/plaincast) released on BSD license by
[Ayke van Laethem](https://aykevl.nl/about).
It uses also a great librarty [kodirpc](https://github.com/pdf/kodirpc) by
Peter Fern.

I would like to thank the creators of
[leapcast](https://github.com/dz0ny/leapcast). Leapcast is a Chromecast
emulator, which was essential in the process of reverse-engineering the YouTube
protocol and better understanding the DIAL protocol.
