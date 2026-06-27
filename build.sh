cd /root/Backhaul-Pro
CGO_ENABLED=0 go build -a -installsuffix cgo -o backhaul-pro .
chmod +x backhaul-pro
cd
echo "Build completed. The executable is named 'backhaul-pro'."
