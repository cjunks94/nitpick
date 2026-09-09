/**
 * Particle Morph Scene — Floating particles that morph between shapes.
 * Circle → Triangle → Square with slow transitions.
 * Mouse hover splits particles apart.
 * Registers with SceneManager.
 */
(function() {
    'use strict';
    if (typeof THREE === 'undefined' || !window.SceneManager) return;

    var PARTICLE_COUNT = 2000;
    var SHAPE_HOLD = 4.5;
    var SHAPE_TRANSITION = 3.0;
    var CYCLE_TIME = SHAPE_HOLD + SHAPE_TRANSITION;
    var SPLIT_RADIUS = 0.15;
    var SPLIT_FORCE = 4.0;
    var FLOAT_AMPLITUDE = 0.4;
    var SHAPE_RADIUS = 10;      // how big the shapes are — fills margins and beyond

    var scene, camera, points;
    var positions, targets, velocities, particleColors;
    var mouse = { x: 0, y: 0 };
    var scrollOffset = 0;
    var shapeTime = 0;

    // Color palettes — picked per active theme bg so particles stay visible on
    // both light and dark surfaces. Selection is driven by --c-bg luminance.
    var DARK_PALETTE = [
        new THREE.Color('#3D6649'),  // forest green (accent)
        new THREE.Color('#5B9BD5'),  // soft blue
        new THREE.Color('#D9534F'),  // muted red
        new THREE.Color('#E8A838'),  // warm amber
        new THREE.Color('#7B68A8'),  // soft purple
        new THREE.Color('#4A7A58'),  // light green
        new THREE.Color('#C0956C'),  // tan
    ];
    var LIGHT_PALETTE = [
        new THREE.Color('#1B4332'),  // deep forest
        new THREE.Color('#1E40AF'),  // royal blue
        new THREE.Color('#991B1B'),  // brick red
        new THREE.Color('#B45309'),  // burnt amber
        new THREE.Color('#5B21B6'),  // deep purple
        new THREE.Color('#15803D'),  // emerald
        new THREE.Color('#78350F'),  // dark brown
    ];

    function getThemeBg() {
        var v = getComputedStyle(document.documentElement).getPropertyValue('--c-bg').trim();
        return v || '#0d1117';
    }

    function isLightBg(hex) {
        var c = (hex || '').trim();
        var r, g, b;
        if (c.charAt(0) === '#') {
            c = c.slice(1);
            if (c.length === 3) c = c[0]+c[0]+c[1]+c[1]+c[2]+c[2];
            if (c.length !== 6) return false;
            r = parseInt(c.slice(0, 2), 16);
            g = parseInt(c.slice(2, 4), 16);
            b = parseInt(c.slice(4, 6), 16);
        } else {
            // Browsers can resolve a custom property to rgb()/rgba() — accept
            // either form so light-theme detection still works.
            var m = c.match(/^rgba?\(\s*([0-9.]+)[,\s]+([0-9.]+)[,\s]+([0-9.]+)/i);
            if (!m) return false;
            r = parseFloat(m[1]); g = parseFloat(m[2]); b = parseFloat(m[3]);
        }
        if (isNaN(r) || isNaN(g) || isNaN(b)) return false;
        // Perceived brightness (Rec.601). >140 ≈ light enough that the dark
        // palette would wash out.
        return (0.299 * r + 0.587 * g + 0.114 * b) > 140;
    }

    function activePalette() {
        return isLightBg(getThemeBg()) ? LIGHT_PALETTE : DARK_PALETTE;
    }

    // ========================================================================
    // SHAPE GENERATORS
    // ========================================================================
    function generateCircle(count, radius) {
        var pts = [];
        for (var i = 0; i < count; i++) {
            var angle = (i / count) * Math.PI * 2;
            var r = radius * (0.5 + Math.random() * 0.5);
            pts.push({
                x: Math.cos(angle) * r,
                y: Math.sin(angle) * r,
                z: (Math.random() - 0.5) * 3
            });
        }
        return pts;
    }

    function generateTriangle(count, size) {
        var pts = [];
        var verts = [
            { x: 0, y: size * 0.85 },
            { x: -size * 0.75, y: -size * 0.55 },
            { x: size * 0.75, y: -size * 0.55 }
        ];
        for (var i = 0; i < count; i++) {
            var edge = i % 3;
            var t = (Math.floor(i / 3) / Math.floor(count / 3));
            t += (Math.random() - 0.5) * 0.08;
            var next = (edge + 1) % 3;
            // Fill interior too — some particles inside
            var fill = Math.random();
            if (fill < 0.3) {
                // Interior point
                var a = Math.random(), b = Math.random();
                if (a + b > 1) { a = 1 - a; b = 1 - b; }
                var c = 1 - a - b;
                pts.push({
                    x: verts[0].x * a + verts[1].x * b + verts[2].x * c,
                    y: verts[0].y * a + verts[1].y * b + verts[2].y * c,
                    z: (Math.random() - 0.5) * 3
                });
            } else {
                pts.push({
                    x: verts[edge].x + (verts[next].x - verts[edge].x) * t,
                    y: verts[edge].y + (verts[next].y - verts[edge].y) * t,
                    z: (Math.random() - 0.5) * 3
                });
            }
        }
        return pts;
    }

    function generateSquare(count, size) {
        var pts = [];
        var half = size * 0.65;
        var verts = [
            { x: -half, y: half },
            { x: half, y: half },
            { x: half, y: -half },
            { x: -half, y: -half }
        ];
        for (var i = 0; i < count; i++) {
            var fill = Math.random();
            if (fill < 0.3) {
                // Interior
                pts.push({
                    x: (Math.random() - 0.5) * size * 1.2,
                    y: (Math.random() - 0.5) * size * 1.2,
                    z: (Math.random() - 0.5) * 3
                });
            } else {
                var edge = i % 4;
                var t = (Math.floor(i / 4) / Math.floor(count / 4));
                t += (Math.random() - 0.5) * 0.08;
                var next = (edge + 1) % 4;
                pts.push({
                    x: verts[edge].x + (verts[next].x - verts[edge].x) * t,
                    y: verts[edge].y + (verts[next].y - verts[edge].y) * t,
                    z: (Math.random() - 0.5) * 3
                });
            }
        }
        return pts;
    }

    var shapes = [generateCircle, generateTriangle, generateSquare];
    var shapeData = [];

    function buildShapeTargets() {
        shapeData = shapes.map(function(fn) { return fn(PARTICLE_COUNT, SHAPE_RADIUS); });
    }

    // ========================================================================
    // SCENE INTERFACE
    // ========================================================================
    window.SceneManager.register('particles', {
        init: function(renderer, canvas) {
            scene = new THREE.Scene();
            scene.background = new THREE.Color(getThemeBg());
            camera = new THREE.PerspectiveCamera(60, window.innerWidth / window.innerHeight, 0.1, 100);
            camera.position.set(0, 0, 14);
            camera.lookAt(0, 0, 0);

            buildShapeTargets();

            positions = new Float32Array(PARTICLE_COUNT * 3);
            targets = new Float32Array(PARTICLE_COUNT * 3);
            velocities = new Float32Array(PARTICLE_COUNT * 3);
            var colorsArr = new Float32Array(PARTICLE_COUNT * 3);

            var firstShape = shapeData[0];
            var palette = activePalette();
            for (var i = 0; i < PARTICLE_COUNT; i++) {
                var i3 = i * 3;
                positions[i3] = firstShape[i].x;
                positions[i3 + 1] = firstShape[i].y;
                positions[i3 + 2] = firstShape[i].z;
                targets[i3] = firstShape[i].x;
                targets[i3 + 1] = firstShape[i].y;
                targets[i3 + 2] = firstShape[i].z;

                // Assign color from active-theme palette
                var col = palette[Math.floor(Math.random() * palette.length)];
                colorsArr[i3] = col.r;
                colorsArr[i3 + 1] = col.g;
                colorsArr[i3 + 2] = col.b;
            }
            particleColors = colorsArr;

            var geometry = new THREE.BufferGeometry();
            geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
            geometry.setAttribute('color', new THREE.BufferAttribute(colorsArr, 3));

            var material = new THREE.PointsMaterial({
                size: 0.12,
                sizeAttenuation: true,
                vertexColors: true,
                transparent: true,
                opacity: 0.85
            });

            points = new THREE.Points(geometry, material);
            scene.add(points);
            shapeTime = 0;
        },

        animate: function(elapsed) {
            if (!points) return;
            shapeTime += 0.016;

            var cyclePos = shapeTime % (CYCLE_TIME * shapes.length);
            var shapeIndex = Math.floor(cyclePos / CYCLE_TIME);
            var timeInCycle = cyclePos - shapeIndex * CYCLE_TIME;
            var nextIndex = (shapeIndex + 1) % shapes.length;

            var morphProgress = 0;
            if (timeInCycle > SHAPE_HOLD) {
                morphProgress = (timeInCycle - SHAPE_HOLD) / SHAPE_TRANSITION;
                morphProgress = morphProgress * morphProgress * (3 - 2 * morphProgress);
            }

            var currentData = shapeData[shapeIndex];
            var nextData = shapeData[nextIndex];

            for (var i = 0; i < PARTICLE_COUNT; i++) {
                var i3 = i * 3;
                targets[i3] = currentData[i].x + (nextData[i].x - currentData[i].x) * morphProgress;
                targets[i3 + 1] = currentData[i].y + (nextData[i].y - currentData[i].y) * morphProgress;
                targets[i3 + 2] = currentData[i].z + (nextData[i].z - currentData[i].z) * morphProgress;
            }

            var mouseWorld = new THREE.Vector3(mouse.x * 12, -mouse.y * 9, 0);

            for (var j = 0; j < PARTICLE_COUNT; j++) {
                var j3 = j * 3;

                var floatX = Math.sin(elapsed * 0.4 + j * 0.08) * FLOAT_AMPLITUDE * 0.4;
                var floatY = Math.cos(elapsed * 0.25 + j * 0.06) * FLOAT_AMPLITUDE * 0.4;

                var tx = targets[j3] + floatX;
                var ty = targets[j3 + 1] + floatY;
                var tz = targets[j3 + 2];

                var dx = positions[j3] - mouseWorld.x;
                var dy = positions[j3 + 1] - mouseWorld.y;
                var dist = Math.sqrt(dx * dx + dy * dy);
                var splitRange = SPLIT_RADIUS * 18;

                if (dist < splitRange && dist > 0.01) {
                    var force = (1 - dist / splitRange) * SPLIT_FORCE;
                    velocities[j3] += (dx / dist) * force * 0.016;
                    velocities[j3 + 1] += (dy / dist) * force * 0.016;
                }

                velocities[j3] *= 0.93;
                velocities[j3 + 1] *= 0.93;
                velocities[j3 + 2] *= 0.93;

                positions[j3] = THREE.MathUtils.lerp(positions[j3], tx, 0.035) + velocities[j3];
                positions[j3 + 1] = THREE.MathUtils.lerp(positions[j3 + 1], ty, 0.035) + velocities[j3 + 1];
                positions[j3 + 2] = THREE.MathUtils.lerp(positions[j3 + 2], tz, 0.035) + velocities[j3 + 2];
            }

            points.geometry.attributes.position.needsUpdate = true;

            camera.position.y = -scrollOffset * 3;
            camera.lookAt(0, -scrollOffset * 3, 0);
        },

        onMouseMove: function(x, y) { mouse.x = x; mouse.y = y; },
        onScroll: function(offset) { scrollOffset = offset; },

        onClick: function(x, y) {
            for (var i = 0; i < PARTICLE_COUNT; i++) {
                var i3 = i * 3;
                var dx = positions[i3] - x * 12;
                var dy = positions[i3 + 1] + y * 9;
                var dist = Math.sqrt(dx * dx + dy * dy) || 1;
                velocities[i3] += (dx / dist) * 0.8;
                velocities[i3 + 1] += (dy / dist) * 0.8;
            }
        },

        resize: function(w, h) { if (camera) { camera.aspect = w / h; camera.updateProjectionMatrix(); } },
        getScene: function() { return scene; },
        getCamera: function() { return camera; },
        updateColors: function() {
            if (!scene || !points || !particleColors) return;
            scene.background = new THREE.Color(getThemeBg());
            var palette = activePalette();
            for (var i = 0; i < PARTICLE_COUNT; i++) {
                var i3 = i * 3;
                var col = palette[Math.floor(Math.random() * palette.length)];
                particleColors[i3]     = col.r;
                particleColors[i3 + 1] = col.g;
                particleColors[i3 + 2] = col.b;
            }
            points.geometry.attributes.color.needsUpdate = true;
        },
        pause: function() { /* shapeTime freezes via manager skipping animate() */ },
        animatePaused: function(elapsed) {
            // Mouse split + velocity only — no shape morphing
            if (!points) return;
            var mouseWorld = new THREE.Vector3(mouse.x * 12, -mouse.y * 9, 0);
            for (var j = 0; j < PARTICLE_COUNT; j++) {
                var j3 = j * 3;
                var dx = positions[j3] - mouseWorld.x;
                var dy = positions[j3 + 1] - mouseWorld.y;
                var dist = Math.sqrt(dx * dx + dy * dy);
                var splitRange = SPLIT_RADIUS * 18;
                if (dist < splitRange && dist > 0.01) {
                    var force = (1 - dist / splitRange) * SPLIT_FORCE;
                    velocities[j3] += (dx / dist) * force * 0.016;
                    velocities[j3 + 1] += (dy / dist) * force * 0.016;
                }
                velocities[j3] *= 0.93;
                velocities[j3 + 1] *= 0.93;
                velocities[j3 + 2] *= 0.93;
                positions[j3] = THREE.MathUtils.lerp(positions[j3], targets[j3], 0.035) + velocities[j3];
                positions[j3 + 1] = THREE.MathUtils.lerp(positions[j3 + 1], targets[j3 + 1], 0.035) + velocities[j3 + 1];
                positions[j3 + 2] = THREE.MathUtils.lerp(positions[j3 + 2], targets[j3 + 2], 0.035) + velocities[j3 + 2];
            }
            points.geometry.attributes.position.needsUpdate = true;
        },
        destroy: function() { scene = null; camera = null; points = null; positions = null; targets = null; velocities = null; }
    });
})();
