# Contributing to go-link

Thanks for wanting to help! go-link is a small, friendly project, and every contribution is welcome: a bug report, an idea, a translation, docs, tests or code.

## Ways to help

- **Star the repository** and share it: it helps other people find go-link.
- **Try it and report what happens.** Open an [issue](https://github.com/lordbasex/go-link-console/issues/new/choose) with what you did, what you expected and what you saw. Your system (macOS, Windows, Linux, Raspberry Pi, Docker), the browser and the version (in the app's window or `go-link-device --help`) help a lot.
- **Ideas and questions** go to [Discussions](https://github.com/lordbasex/go-link-console/discussions).
- **Translations:** the website's texts live in `frontend/apps/web/src/i18n/`. `en.ts` is the reference; each language is one file with the same shape (see `es.ts` and `pt.ts`), plus the user guide (`docs-*.ts`) and the legal pages (`legal-*.ts`). The app window's texts are in `backend-device/internal/gui/i18n_*.go`.
- **Code, docs and tests:** see below.

## Getting started

Requirements: Go, Node.js, `pkg-config`, libvpx and Opus (on macOS: `brew install libvpx opus pkg-config`). The full list and the build targets are in [docs/building.md](docs/building.md).

```bash
# The device (uses the public signaling server)
cd backend-device
go run ./cmd/device --web-url http://localhost:5180

# The website
cd frontend
npm install
npm run dev            # http://localhost:5180
```

Open `http://localhost:5180/device` and type the code shown in the device window. [docs/README.md](docs/README.md) explains the architecture, the protocols and where everything lives.

## Before you open a pull request

- **One change per pull request,** with a short description of what and why. For something big, open an issue or a discussion first so we can agree on the approach.
- **Everything in the repository is in English:** code, comments, identifiers, messages, commit messages and docs. The only exceptions are the translated strings in the `i18n` files.
- **Every source file starts with the copyright line** used in the other files of its folder.
- **Run the checks** that CI runs:
  - Device (`backend-device/`): `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`
  - Website (`frontend/`): `npm run typecheck`, `npm test`
  - MP4 helper (`frontend/wasm/mp4/`): `go test -race ./...`
- **Keep the docs in sync:** user-facing changes go in the user guide (`frontend/apps/web/src/i18n/docs-*.ts`), the developer docs (`docs/`) and `CHANGELOG.md` under `[Unreleased]`.
- **ROMs:** never add, link to or name ROM or game image websites, and never add code that downloads ROMs. go-link only plays the host's own files.

## Code of conduct

Be kind and patient. We are all here to play and to build something fun together. Harassment or disrespect of any kind is not welcome.

## License

By contributing you agree that your contribution is released under the [MIT license](LICENSE) of this repository.
