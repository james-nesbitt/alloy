// Alloy Bridge for WASM-to-JS & JS-to-Go
"use strict";

const bridge = {
    // Current application state
    state: {
        mode: 'Normal',
        query: '',
        results: [],
        selectedIndex: 0,
        connected: false
    },

    // Initialize the web frontend
    init: async () => {
        console.log("Alloy: Initializing Web Bridge...");
        this.setupKeyboard();
        this.setupSSE();

        // Check for WASM availability
        if (typeof Go === 'undefined') {
            console.error("Alloy: Go WASM support missing/not loaded.");
            this.updateStatus('KERNEL', 'crashed');
            return;
        }

        const go = new Go();
        try {
            const result = await WebAssembly.instantiateStreaming(
                fetch("/static/wasm/frontend.wasm"), 
                go.importObject
            );
            go.run(result.instance);
            console.log("Alloy: WASM Frontend Initialized.");
            this.updateStatus('KERNEL', 'online');
        } catch (err) {
            console.error("Alloy: Failed to load WASM frontend:", err);
            this.updateStatus('KERNEL', 'offline');
        }
    },

    // Handle incoming events from the Kernel
    setupSSE: () => {
        const eventSource = new EventSource("/api/events");
        eventSource.onmessage = (event) => {
            const data = JSON.parse(event.data);
            if (data.type === 'keepalive') return;
            
            // Dispatch to WASM or handle in JS
            console.log("Kernel Event:", data);
        };
        eventSource.onerror = (e) => {
            console.error("SSE Connection Lost:", e);
        };
    },

    // Listen for global hotkeys (Omni-palette, navigation)
    setupKeyboard: () => {
        const omniOverlay = document.getElementById('command-overlay');
        const omniInput = document.getElementById('omni-input');

        window.addEventListener('keydown', (e) => {
            // Toggle Omni-palette with F1 or ':'
            if (e.key === 'F1' || (e.key === ':' && document.activeElement !== omniInput)) {
                e.preventDefault();
                this.toggleOmni();
            }

            // Dismiss with Escape
            if (e.key === 'Escape' && this.state.mode === 'Command') {
                this.toggleOmni(false);
            }

            // Navigate results with Arrow Keys
            if (this.state.mode === 'Command') {
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    this.navigateResults(1);
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    this.navigateResults(-1);
                } else if (e.key === 'Enter') {
                    e.preventDefault();
                    this.selectResult();
                }
            }
        });

        omniInput.addEventListener('input', (e) => {
            this.state.query = e.target.value;
            this.filterResults();
        });
    },

    toggleOmni: (show) => {
        const overlay = document.getElementById('command-overlay');
        const input = document.getElementById('omni-input');
        
        if (show === undefined) show = overlay.classList.contains('hidden');

        if (show) {
            overlay.classList.remove('hidden');
            overlay.classList.add('flex');
            input.focus();
            this.state.mode = 'Command';
        } else {
            overlay.classList.add('hidden');
            overlay.classList.remove('flex');
            this.state.mode = 'Normal';
        }
    },

    updateStatus: (component, status) => {
        const dot = document.getElementById(`${component.toLowerCase()}-dot`);
        if (!dot) return;

        dot.className = "w-2 h-2 rounded-full";
        switch(status) {
            case 'online': dot.classList.add('bg-emerald-500'); break;
            case 'offline': dot.classList.add('bg-zinc-600'); break;
            case 'crashed': dot.classList.add('bg-rose-500'); break;
            case 'loading': dot.classList.add('bg-amber-500'); break;
        }
    },

    // Dummy logic for prototype rendering - eventually handled via WASM bridge
    filterResults: () => {
        const list = document.getElementById('omni-results');
        list.innerHTML = `<li class="px-4 py-2 text-zinc-500 italic">Searching for "${this.state.query}"...</li>`;
    }
};

window.alloy = bridge;
document.addEventListener('DOMContentLoaded', () => bridge.init());
