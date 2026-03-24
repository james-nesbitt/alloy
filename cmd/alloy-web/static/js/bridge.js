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
        bridge.setupKeyboard();
        bridge.setupSSE();

        // Check for WASM availability
        if (typeof Go === 'undefined') {
            console.error("Alloy: Go WASM support missing/not loaded.");
            bridge.updateStatus('KERNEL', 'crashed');
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
            bridge.updateStatus('KERNEL', 'online');
            
            // Initial empty search to populate results
            bridge.filterResults();
        } catch (err) {
            console.error("Alloy: Failed to load WASM frontend:", err);
            bridge.updateStatus('KERNEL', 'offline');
        }
    },

    // Handle incoming events from the Kernel
    setupSSE: () => {
        const eventSource = new EventSource("/api/events");
        eventSource.onmessage = (event) => {
            const data = JSON.parse(event.data);
            if (data.type === 'keepalive') return;
            
            // Dispatch to WASM
            if (window.alloy && window.alloy.handleEvent) {
                window.alloy.handleEvent(event.data);
            }
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
                bridge.toggleOmni();
            }

            // Dismiss with Escape
            if (e.key === 'Escape' && bridge.state.mode === 'Command') {
                bridge.toggleOmni(false);
            }

            // Navigate results with Arrow Keys
            if (bridge.state.mode === 'Command') {
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    bridge.navigateResults(1);
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    bridge.navigateResults(-1);
                } else if (e.key === 'Enter') {
                    e.preventDefault();
                    bridge.selectResult();
                }
            }
        });

        omniInput.addEventListener('input', (e) => {
            bridge.state.query = e.target.value;
            bridge.filterResults();
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
            bridge.state.mode = 'Command';
        } else {
            overlay.classList.add('hidden');
            overlay.classList.remove('flex');
            bridge.state.mode = 'Normal';
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

    navigateResults: (dir) => {
        bridge.state.selectedIndex += dir;
        const max = bridge.state.results.length - 1;
        if (bridge.state.selectedIndex < 0) bridge.state.selectedIndex = max;
        if (bridge.state.selectedIndex > max) bridge.state.selectedIndex = 0;
        bridge.renderResults(bridge.state.results);
    },

    selectResult: () => {
        const res = bridge.state.results[bridge.state.selectedIndex];
        if (!res) return;
        
        console.log("Selecting result:", res);
        if (window.alloy && window.alloy.send) {
            window.alloy.send(res.target, res.method);
        }
        bridge.toggleOmni(false);
    },

    // Use the WASM bridge to search
    renderResults: (results) => {
        const list = document.getElementById('omni-results');
        if (!results || results.length === 0) {
            list.innerHTML = `<li class="px-4 py-2 text-zinc-500 italic">No matches found for "${bridge.state.query}".</li>`;
            return;
        }

        list.innerHTML = results.map((res, i) => `
            <li class="px-4 py-2 hover:bg-zinc-800 flex items-center justify-between group cursor-pointer ${i === bridge.state.selectedIndex ? 'bg-zinc-800' : ''}">
                <div class="flex items-center space-x-3">
                    <span class="px-2 py-0.5 bg-zinc-700 text-zinc-400 text-[10px] rounded uppercase font-bold tracking-tight">${res.group || res.target}</span>
                    <span class="text-zinc-100">${res.full_title}</span>
                </div>
                <div class="flex items-center space-x-4 opacity-0 group-hover:opacity-100 transition-opacity">
                    <span class="text-zinc-500 text-xs italic">${res.description}</span>
                </div>
            </li>
        `).join('');
    },

    filterResults: () => {
        if (!window.alloy || !window.alloy.search) return;
        
        try {
            const resJson = window.alloy.search(bridge.state.query);
            bridge.state.results = JSON.parse(resJson);
            bridge.renderResults(bridge.state.results);
        } catch (err) {
            console.error("Search error:", err);
        }
    }
};

window.alloy = bridge;
document.addEventListener('DOMContentLoaded', () => bridge.init());
