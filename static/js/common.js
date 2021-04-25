function highlightCipher(id) {
	$("#cipher-list .cipher").removeClass("highlight");
	$(".cipher#cipher-" + id).addClass("highlight").scrollintoview();
}

// MAP FUNCTIONS:

if (typeof L !== 'undefined') {
	var teamIcon = L.ExtraMarkers.icon({
		icon: 'fa-street-view',
		shape: 'circle',
		markerColor: 'blue',
		svg: true,
		prefix: 'fa'
	});
	var cipherIcon = L.ExtraMarkers.icon({
		icon: 'fa-file',
		shape: 'penta',
		markerColor: 'yellow',
		svg: true,
		prefix: 'fa'
	});
}
