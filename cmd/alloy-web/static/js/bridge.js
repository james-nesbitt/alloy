// Alloy Bridge for WASM-to-JS & JS-to-Go
"use strict";

(function() {
    const bridge = {
        // Current application state
        state: {
            mode: 'Normal', // Normal, Command, Form
            query: '',
            results: [],
            selectedIndex: 0,
            connected: false,
            activeCommand: null,
            formData: {},
            searchTimeout: null
        },

        // Initialize the web frontend
        init: async () => {
            console.log("Alloy: Initializing Web Bridge...");
            bridge.setupKeyboard();
            bridge.setupWS();
            bridge.setupFormActions();

            // Subscribe to important events
            setTimeout(() => {
                const topics = [
                    "project:opened", "workspace:opened", "workspace:set",
                    "presence:online", "presence:offline", "presence:heartbeat",
                    "buffer:update", "buffer:cursors_updated",
                    "dashboard:widget-registered", "dashboard:widget-updated",
                    "system:trace", "system:context-changed", "system:theme-changed"
                ];
                topics.forEach(topic => {
                    bridge.executeCommand('events', 'subscribe', { topic: topic });
                });
            }, 1000);


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

        // Handle incoming messages from the Kernel via WebSocket
        setupWS: () => {
            const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const ws = new WebSocket(`${proto}//${window.location.host}/ws`);
            bridge.socket = ws;

            ws.onopen = () => {
                console.log("WebSocket Connection Opened");
                bridge.updateStatus('KERNEL', 'online');
                bridge.state.connected = true;
            };

            ws.onmessage = (event) => {
                const data = JSON.parse(event.data);
                
                // Handle Omni Search results
                if (data.method === 'omni:search' || data.id?.startsWith('omni-search-')) {
                    let results = data.payload;
                    if (typeof results === 'string') {
                        try {
                            results = JSON.parse(results);
                        } catch (e) {
                            console.error("Failed to parse omni results", e);
                            return;
                        }
                    }
                    
                    // Map results to the format bridge expects
                    bridge.state.results = results.map(r => ({
                        target: (r.type === 'command') ? r.id : (r.source || r.type),
                        method: r.id,
                        full_title: r.title,
                        description: r.description,
                        metadata: r.metadata,
                        params: [] // WASM plugins don't provide param metadata yet in this view
                    }));
                    bridge.state.selectedIndex = 0;
                    bridge.renderResults(bridge.state.results);
                    return;
                }

                // Handle presence and cursor events specifically
                if (data.type === 'event' && data.sender === 'events') {
                    const topic = data.method;
                    if (topic === 'buffer:cursors_updated' || topic === 'buffer:update') {
                        const evt = typeof data.payload === 'string' ? JSON.parse(data.payload) : data.payload;
                        console.log("Buffer Event:", topic, evt);
                        // If it's a cursor update, we should probably re-fetch the buffer presence
                        // for now just log it
                    }
                }

                // Handle published events from the events plugin

                if (data.type === 'event' && data.sender === 'events') {
                    const topic = data.method;
                    const payload = typeof data.payload === 'string' ? JSON.parse(data.payload) : data.payload;

                    if (topic === 'dashboard:widget-registered') {
                        bridge.registerWidget(payload);
                        return;
                    }
                    if (topic === 'dashboard:widget-updated') {
                        bridge.updateWidget(data.metadata?.widget_id, payload);
                        return;
                    }
                    if (topic === 'project:opened' || topic === 'workspace:opened') {
                        const evtData = payload.data || payload;
                        if (evtData.layout && evtData.layout.root) {
                            bridge.state.rootLayout = evtData.layout.root;
                            bridge.renderLayout();
                        } else if (evtData.Layout && typeof evtData.Layout === 'string') {
                            // Workspace layout string
                            try {
                                const wcfg = JSON.parse(evtData.Layout);
                                if (wcfg.root) {
                                    bridge.state.rootLayout = wcfg.root;
                                    bridge.renderLayout();
                                }
                            } catch(e) {}
                        }
                        return;
                    }
                }
                
                // Dispatch to WASM logic
                if (window.alloy && window.alloy.handleEvent) {
                    window.alloy.handleEvent(event.data);
                }
            };

            ws.onerror = (e) => {
                console.error("WebSocket Error:", e);
                bridge.updateStatus('KERNEL', 'offline');
                bridge.state.connected = false;
            };

            ws.onclose = () => {
                console.log("WebSocket Connection Closed");
                bridge.updateStatus('KERNEL', 'offline');
                bridge.state.connected = false;
                // Retry after delay
                setTimeout(bridge.setupWS, 5000);
            };
        },

        setupFormActions: () => {
            document.getElementById('form-cancel').onclick = () => bridge.setMode('Command');
            document.getElementById('form-submit').onclick = () => bridge.submitForm();
        },

        // Listen for global hotkeys (Omni-palette, navigation)
        setupKeyboard: () => {
            const omniInput = document.getElementById('omni-input');

            window.addEventListener('keydown', (e) => {
                // Toggle Omni-palette with F1 or ':'
                if ((e.key === 'F1' || e.key === ':') && bridge.state.mode === 'Normal' && document.activeElement.tagName !== 'INPUT') {
                    e.preventDefault();
                    bridge.toggleOmni(true);
                }

                // Global Escape handling
                if (e.key === 'Escape') {
                    if (bridge.state.mode === 'Form') {
                        bridge.setMode('Command');
                    } else if (bridge.state.mode === 'Command') {
                        bridge.toggleOmni(false);
                    }
                }

                // Command mode navigation
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

                // Form mode submit on Enter (if not in textarea)
                if (bridge.state.mode === 'Form' && e.key === 'Enter' && e.target.tagName !== 'TEXTAREA') {
                    bridge.submitForm();
                }
            });

            omniInput.addEventListener('input', (e) => {
                bridge.state.query = e.target.value;
                bridge.filterResults();
            });
        },

        setMode: (mode) => {
            bridge.state.mode = mode;
            const searchUI = document.getElementById('omni-search-container');
            const resultsUI = document.getElementById('omni-results');
            const formUI = document.getElementById('omni-form');
            const searchHints = document.getElementById('hints-search');
            const formHints = document.getElementById('hints-form');

            if (mode === 'Command') {
                searchUI.classList.remove('hidden');
                resultsUI.classList.remove('hidden');
                formUI.classList.add('hidden');
                searchHints.classList.remove('hidden');
                formHints.classList.add('hidden');
                document.getElementById('omni-input').focus();
            } else if (mode === 'Form') {
                searchUI.classList.add('hidden');
                resultsUI.classList.add('hidden');
                formUI.classList.remove('hidden');
                searchHints.classList.add('hidden');
                formHints.classList.remove('hidden');
                
                // Focus first input
                const firstInput = formUI.querySelector('input, select, textarea');
                if (firstInput) firstInput.focus();
            } else {
                bridge.toggleOmni(false);
            }
        },

        toggleOmni: (show) => {
            const overlay = document.getElementById('command-overlay');
            const input = document.getElementById('omni-input');
            
            if (show === undefined) show = overlay.classList.contains('hidden');

            if (show) {
                overlay.classList.remove('hidden');
                overlay.classList.add('flex');
                bridge.setMode('Command');
                input.value = "";
                bridge.state.query = "";
                bridge.filterResults();
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

            // Ensure selected item is visible
            const selected = document.querySelector('#omni-results li.bg-zinc-800');
            if (selected && selected.scrollIntoView) selected.scrollIntoView({ block: 'nearest' });
        },

        selectResult: () => {
            const res = bridge.state.results[bridge.state.selectedIndex];
            if (!res) return;

            // Handle metadata-based actions
            if (res.metadata && res.metadata.action) {
                const action = res.metadata.action;
                console.log("Omni Action:", action, res.metadata);

                if (action === 'switch' && res.metadata.buffer_id) {
                    bridge.executeCommand('buffer', 'buffer:read', { id: res.metadata.buffer_id });
                    bridge.toggleOmni(false);
                    return;
                }
                
                if (action === 'open' && res.metadata.path) {
                    // Logic to open a document
                    bridge.notify(`Opening Document: ${res.metadata.path}`, 'info');
                    bridge.toggleOmni(false);
                    return;
                }
            }
            
            if (res.params && res.params.length > 0) {
                bridge.showForm(res);
            } else {
                bridge.executeCommand(res.target, res.method, {});
                bridge.toggleOmni(false);
            }
        },

        showForm: (cmd) => {
            bridge.state.activeCommand = cmd;
            bridge.state.mode = 'Form';
            
            document.getElementById('form-title').innerText = cmd.full_title;
            document.getElementById('form-description').innerText = cmd.description || "Enter parameters for this command.";
            
            const fieldsContainer = document.getElementById('form-fields');
            fieldsContainer.innerHTML = '';
            
            cmd.params.forEach(param => {
                const field = document.createElement('div');
                field.className = 'flex flex-col space-y-1';
                
                const label = document.createElement('label');
                label.className = 'text-[10px] text-zinc-500 uppercase font-bold tracking-widest';
                label.innerText = param.name;
                field.appendChild(label);
                
                let input;
                if (param.type === 'string' && param.description.toLowerCase().includes('long')) {
                    input = document.createElement('textarea');
                    input.className = 'bg-zinc-850 border border-zinc-700 rounded p-2 text-zinc-200 focus:border-indigo-500 focus:outline-none min-h-[100px] text-sm';
                } else {
                    input = document.createElement('input');
                    input.type = param.type === 'int' ? 'number' : 'text';
                    input.className = 'bg-zinc-850 border border-zinc-700 rounded p-2 text-zinc-200 focus:border-indigo-500 focus:outline-none text-sm';
                }
                
                input.id = `param-${param.name}`;
                input.placeholder = param.description || "";
                if (param.default) input.value = param.default;
                
                field.appendChild(input);
                fieldsContainer.appendChild(field);
            });
            
            bridge.setMode('Form');
        },

        submitForm: () => {
            const cmd = bridge.state.activeCommand;
            if (!cmd) return;
            
            const payload = {};
            cmd.params.forEach(param => {
                const input = document.getElementById(`param-${param.name}`);
                if (!input) return;
                let val = input.value;
                if (param.type === 'int') val = parseInt(val, 10);
                if (param.type === 'bool') val = input.checked;
                payload[param.name] = val;
            });
            
            bridge.executeCommand(cmd.target, cmd.method, payload);
            bridge.toggleOmni(false);
        },

        executeCommand: (target, method, payload) => {
            console.log("Executing command via WebSocket:", {target, method, payload});
            
            const msg = {
                id: `web-${Date.now()}`,
                type: 'request',
                sender: 'web-client',
                target: target,
                method: method,
                payload: typeof payload === 'string' ? payload : JSON.stringify(payload)
            };

            if (bridge.socket && bridge.socket.readyState === WebSocket.OPEN) {
                bridge.socket.send(JSON.stringify(msg));
                bridge.notify(`Command sent: ${target}:${method}`, 'success');
            } else {
                bridge.notify(`WebSocket not connected, failed to send ${target}:${method}`, 'error');
            }
        },

        notify: (msg, type = 'info') => {
            const container = document.getElementById('notifications');
            const n = document.createElement('div');
            n.className = `px-4 py-2 rounded shadow-lg text-sm font-bold border-l-4 transition-all transform translate-x-full duration-300 opacity-0`;
            
            if (type === 'success') n.classList.add('bg-zinc-900', 'text-emerald-400', 'border-emerald-500');
            else if (type === 'error') n.classList.add('bg-zinc-900', 'text-rose-400', 'border-rose-500');
            else n.classList.add('bg-zinc-900', 'text-zinc-200', 'border-zinc-500');
            
            n.innerText = msg;
            container.appendChild(n);
            
            // Animate in
            setTimeout(() => {
                n.classList.remove('translate-x-full', 'opacity-0');
            }, 10);
            
            // Remove after delay
            setTimeout(() => {
                n.classList.add('translate-x-full', 'opacity-0');
                setTimeout(() => n.remove(), 300);
            }, 4000);
        },

        // Use the WASM bridge to search
        renderResults: (results) => {
            const list = document.getElementById('omni-results');
            if (!results || results.length === 0) {
                list.innerHTML = `<li class="px-4 py-2 text-zinc-500 italic">No matches found for "${bridge.state.query}".</li>`;
                return;
            }

            list.innerHTML = results.map((res, i) => `
                <li class="px-4 py-3 hover:bg-zinc-800 flex items-center justify-between group cursor-pointer ${i === bridge.state.selectedIndex ? 'bg-zinc-800' : ''}" onclick="window.alloy.selectedIndex=${i}; window.alloy.selectResult();">
                    <div class="flex items-center space-x-3 overflow-hidden">
                        <span class="flex-shrink-0 px-2 py-0.5 bg-zinc-700 text-zinc-400 text-[10px] rounded uppercase font-bold tracking-tight">${res.group || res.target}</span>
                        <span class="text-zinc-100 font-medium truncate">${res.full_title}</span>
                    </div>
                    <div class="flex items-center space-x-4 ml-4 flex-shrink-0 opacity-0 group-hover:opacity-100 ${i === bridge.state.selectedIndex ? 'opacity-100' : ''} transition-opacity">
                        <span class="text-zinc-500 text-xs italic">${res.description || ""}</span>
                        ${res.params && res.params.length > 0 ? '<span class="text-[9px] bg-indigo-900/50 text-indigo-300 px-1 border border-indigo-700/50 rounded">FORM</span>' : ''}
                    </div>
                </li>
            `).join('');
        },

        filterResults: () => {
            // DEBOUNCED SEARCH via WebSocket
            if (bridge.state.searchTimeout) clearTimeout(bridge.state.searchTimeout);

            bridge.state.searchTimeout = setTimeout(() => {
                const query = bridge.state.query;
                
                const msg = {
                    id: `omni-search-${Date.now()}`,
                    type: 'request',
                    sender: 'web-client',
                    target: 'omni-palette',
                    method: 'omni:search',
                    payload: JSON.stringify({ query: query, limit: 15 })
                };

                if (bridge.socket && bridge.socket.readyState === WebSocket.OPEN) {
                    bridge.socket.send(JSON.stringify(msg));
                }
            }, 150);
        },

        updateWidget: (id, content) => {
            console.log("Updating widget:", id);
            const widgetEl = document.querySelector(`[data-widget-id="${id}"] .widget-content`);
            if (widgetEl) {
                // If content is base64 (from WASM), decode it
                if (typeof content === 'string' && /^[A-Za-z0-9+/=]+$/.test(content)) {
                    try {
                        widgetEl.innerHTML = atob(content);
                    } catch(e) {
                        widgetEl.innerText = content;
                    }
                } else {
                    widgetEl.innerText = content;
                }
            }
        },

        registerWidget: (widget) => {
            console.log("Registering widget:", widget.id);
            if (!bridge.state.widgets) bridge.state.widgets = {};
            bridge.state.widgets[widget.id] = widget;

            // If we don't have a specific layout, we just append to a default grid
            if (!bridge.state.rootLayout) {
                const container = document.getElementById('dashboard');
                if (!container) return;

                if (document.querySelector(`[data-widget-id="${widget.id}"]`)) return;

                const el = bridge.createWidgetElement(widget);
                container.appendChild(el);
            } else {
                bridge.renderLayout();
            }
        },

        createWidgetElement: (widget) => {
            const el = document.createElement('div');
            el.className = 'bg-zinc-900 border border-zinc-800 rounded-lg p-4 flex flex-col space-y-2 h-full';
            el.setAttribute('data-widget-id', widget.id);
            
            const title = document.createElement('h3');
            title.className = 'text-zinc-400 text-xs font-bold uppercase tracking-widest';
            title.innerText = widget.title;
            
            const content = document.createElement('div');
            content.className = 'text-zinc-100 widget-content overflow-auto flex-grow';
            content.innerText = 'Loading...';
            
            el.appendChild(title);
            el.appendChild(content);
            
            if (widget.content) {
                // ... internal update logic
                setTimeout(() => bridge.updateWidget(widget.id, widget.content), 10);
            }
            return el;
        },

        renderLayout: () => {
            const container = document.getElementById('dashboard');
            if (!container || !bridge.state.rootLayout) return;

            // Change dashboard from grid to flex for recursive layout
            container.className = "flex w-full h-full p-0 gap-0 overflow-hidden";
            container.innerHTML = "";
            
            const rootEl = bridge.renderLayoutNode(bridge.state.rootLayout);
            container.appendChild(rootEl);
        },

        renderLayoutNode: (node) => {
            const el = document.createElement('div');
            el.style.display = 'flex';
            el.style.flex = node.weight || 1;
            el.style.overflow = 'hidden';
            el.style.boxSizing = 'border-box';

            if (node.type === 'pane') {
                el.classList.add('p-1');
                if (node.plugin_id && bridge.state.widgets && bridge.state.widgets[node.plugin_id]) {
                    el.appendChild(bridge.createWidgetElement(bridge.state.widgets[node.plugin_id]));
                } else {
                    const placeholder = document.createElement('div');
                    placeholder.className = 'bg-zinc-950 border border-zinc-800 rounded-lg p-4 flex items-center justify-center text-zinc-600 italic h-full w-full';
                    placeholder.innerText = `Pane: ${node.mode || 'empty'}`;
                    el.appendChild(placeholder);
                }
                return el;
            }

            // Split
            el.style.flexDirection = node.direction === 'vertical' ? 'column' : 'row';
            if (node.children) {
                node.children.forEach(child => {
                    el.appendChild(bridge.renderLayoutNode(child));
                });
            }
            return el;
        }
    };

    window.alloy = bridge;
    if (typeof document !== 'undefined') {
        document.addEventListener('DOMContentLoaded', () => bridge.init());
    }
})();
