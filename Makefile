NAME := shout

LISP     := sbcl
LISPOPTS := --no-sysinit --no-userinit
LISPEXEC := $(LISP) --script

BUILD := build
BUILDAPP = $(BUILD)/buildapp

QLDIR   := $(BUILD)/quicklisp
QLURL   := http://beta.quicklisp.org/quicklisp.lisp
QLFILE  := $(QLDIR)/quicklisp.lisp
QLSETUP := $(QLDIR)/setup.lisp

LISP_QL := $(LISP) $(LISPOPTS) --load $(QLSETUP)

PREFIX = /usr/local
INSTALL_DIR = $(DESTDIR)$(PREFIX)/bin

default: all

# Use vendored quicklisp if available, otherwise download
$(QLDIR)/setup.lisp:
	@if [ -d vendor/quicklisp ]; then \
		echo "Using vendored Quicklisp dependencies"; \
		mkdir -p $(BUILD); \
		cp -r vendor/quicklisp $(BUILD)/quicklisp; \
	else \
		echo "Install Quicklisp (downloading)"; \
		mkdir -p $(QLDIR); \
		curl -o $(QLFILE) $(QLURL); \
		$(LISP) $(LISPOPTS) --load $(QLFILE) \
		  --eval '(quicklisp-quickstart:install :path "$(QLDIR)")' \
		  --quit; \
		rm $(QLFILE); \
	fi

quicklisp: $(QLDIR)/setup.lisp ;

$(BUILD)/.reqs: $(QLDIR)/setup.lisp
	mkdir -p $(BUILD)
	@if [ -d vendor/quicklisp ]; then \
		echo "Dependencies already vendored"; \
	else \
		echo "Downloading requirements"; \
		$(LISP_QL) --eval '(ql:quickload :hunchentoot)'  \
		           --eval '(ql:quickload :drakma)'       \
		           --eval '(ql:quickload :cl-json)'      \
		           --eval '(ql:quickload :daemon)'       \
		           --eval '(ql:quickload :prove)'        \
		           --eval '(load "$(NAME).asd")' --quit; \
	fi
	touch $@

libs: $(BUILD)/.reqs ;

all: shout
shout: quicklisp libs
	$(LISPEXEC) compile.lisp

test: quicklisp libs
	$(LISPEXEC) test.lisp
coverage: quicklisp libs
	$(LISPEXEC) cover.lisp

# Refresh vendored dependencies (run on connected machine)
vendor: quicklisp libs
	rm -rf vendor/quicklisp
	cp -r $(BUILD)/quicklisp vendor/quicklisp

docker:
	docker build \
	  --build-arg BUILD_DATE="$(shell date -u -Iminutes)" \
	  --build-arg VCS_REF="$(shell git rev-parse --short HEAD)" \
	  --platform linux/amd64 \
	  -t genesiscommunity/shout:ubuntu-jammy .

clean:
	rm -rf $(BUILD)

.PHONY: all test clean vendor
