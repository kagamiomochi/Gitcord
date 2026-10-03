sudo pacman -S --needed webkit2gtk-4.1 base-devel curl wget file openssl libappindicator-gtk3 librsvg xdotool go rust nodejs npm
TRIPLE=$(rustc -vV | sed -n 's/host: //p')
mkdir -p src-tauri/binaries
CGO_ENABLED=0 go build -ldflags="-s -w" -o src-tauri/binaries/gitcord-server-$TRIPLE .
npm install
npx tauri build
