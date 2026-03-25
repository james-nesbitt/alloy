/**
 * @vitest-environment jsdom
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readFileSync } from 'fs';
import { resolve } from 'path';

// Helper to load and execute the bridge.js in the JSDOM
function loadBridge() {
    const bridgePath = resolve(__dirname, './bridge.js');
    const bridgeJs = readFileSync(bridgePath, 'utf8');
    // Execute bridge script
    new Function(bridgeJs).call(window);
}

const HTML_CONTENT = `
    <div id="status-indicators">
        <span id="kernel-dot"></span>
    </div>
    <div id="command-overlay" class="hidden">
        <div id="omni-panel">
            <div id="omni-search-container">
                <input type="text" id="omni-input">
            </div>
            <ul id="omni-results"></ul>
            <div id="omni-form" class="hidden">
                <h2 id="form-title"></h2>
                <p id="form-description"></p>
                <div id="form-fields"></div>
                <button id="form-cancel"></button>
                <button id="form-submit"></button>
            </div>
            <div id="omni-hints">
                <div id="hints-search"></div>
                <div id="hints-form" class="hidden"></div>
            </div>
        </div>
    </div>
    <div id="notifications"></div>
`;

describe('Alloy Web UX - Bridge.js', () => {
    beforeEach(async () => {
        document.body.innerHTML = HTML_CONTENT;
        
        // Mock global window objects
        window.Go = class { importObject = {}; run() {} };
        window.WebAssembly = {
            instantiateStreaming: vi.fn().mockResolvedValue({ instance: { exports: {} } })
        };
        window.EventSource = vi.fn().mockImplementation(() => ({
            onmessage: null,
            onerror: null
        }));
        
        // Mock fetch for WASM and for commands
        window.fetch = vi.fn().mockImplementation((url) => {
             if (url.includes('frontend.wasm')) return Promise.resolve(new Response());
             return Promise.resolve(new Response("[]"));
        });
        
        // Load the bridge
        loadBridge();

        // Initialize explicitly for testing
        await window.alloy.init();
    });

    it('should initialize and register global alloy object', async () => {
        expect(window.alloy).toBeDefined();
        expect(window.alloy.state.mode).toBe('Normal');
    });

    it('should toggle omni-palette when toggleOmni is called', () => {
        const overlay = document.getElementById('command-overlay');
        
        window.alloy.toggleOmni(true);
        expect(overlay.classList.contains('hidden')).toBe(false);
        expect(window.alloy.state.mode).toBe('Command');

        window.alloy.toggleOmni(false);
        expect(overlay.classList.contains('hidden')).toBe(true);
        expect(window.alloy.state.mode).toBe('Normal');
    });

    it('should show form when a command with params is selected', () => {
        const cmdWithParams = {
            target: 'project',
            method: 'create',
            full_title: 'Project: Create',
            description: 'Create a new project',
            params: [
                { name: 'name', type: 'string', description: 'Project name' }
            ]
        };

        window.alloy.showForm(cmdWithParams);

        expect(window.alloy.state.mode).toBe('Form');
        expect(document.getElementById('omni-form').classList.contains('hidden')).toBe(false);
        expect(document.getElementById('form-title').innerText).toBe('Project: Create');
        
        const fields = document.getElementById('form-fields');
        expect(fields.children.length).toBe(1);
        expect(fields.querySelector('input')).toBeDefined();
        expect(fields.querySelector('label').innerText).toBe('name');
    });

    it('should search using the WASM bridge', () => {
        // Mock the window.alloy.search function that WASM provides
        window.alloy.search = vi.fn().mockReturnValue(JSON.stringify([
            { full_title: 'project:open', target: 'project', method: 'open' }
        ]));

        const input = document.getElementById('omni-input');
        input.value = 'proj';
        input.dispatchEvent(new Event('input'));

        expect(window.alloy.search).toHaveBeenCalledWith('proj');
        const results = document.getElementById('omni-results');
        expect(results.children.length).toBe(1);
        expect(results.innerHTML).toContain('project:open');
    });

    it('should navigate results using Arrow keys', () => {
        window.alloy.state.results = [
            { full_title: 'cmd1' },
            { full_title: 'cmd2' }
        ];
        window.alloy.renderResults(window.alloy.state.results);
        window.alloy.state.mode = 'Command';

        // Initial index 0
        expect(window.alloy.state.selectedIndex).toBe(0);

        // ArrowDown
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown' }));
        expect(window.alloy.state.selectedIndex).toBe(1);

        // ArrowUp
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp' }));
        expect(window.alloy.state.selectedIndex).toBe(0);
    });
});
