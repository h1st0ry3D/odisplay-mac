update:
	git pull && ./build/build.sh
	rm -rf /Applications/Odisplay.app
	mv build/Odisplay.app /Applications

open: 
	open /Applications/Odisplay.app	